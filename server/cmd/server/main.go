package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"wendaoiotpanel/internal/config"
	"wendaoiotpanel/internal/events"
	"wendaoiotpanel/internal/handler"
	"wendaoiotpanel/internal/model"
	mqttclient "wendaoiotpanel/internal/mqtt"
	"wendaoiotpanel/internal/store"
	"wendaoiotpanel/internal/syncapi"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config invalid: %v", err)
	}
	log.Printf("config loaded: server.port=%d mysql=%s:%d/%s mqtt=%s",
		cfg.Server.Port, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database, cfg.MQTT.Broker)

	handler.InitJWTSecret(cfg.Server.JWT)

	s, err := store.New(cfg.MySQLDSN())
	if err != nil {
		log.Fatalf("connect mysql: %v", err)
	}
	if err := s.AutoMigrate(); err != nil {
		log.Fatalf("auto migrate: %v", err)
	}

	bus := events.NewBus()

	// 平台业务 MQTT 客户端：以普通客户端身份连接独立 broker 服务（平台账号分支放行）。
	// broker 可能晚于后端启动，这里做有限重试，避免依赖容器启动顺序的脆弱性。
	mc, err := mqttclient.New(mqttclient.Config{
		Broker:   cfg.MQTT.Broker,
		ClientID: cfg.MQTT.ClientID,
		Username: cfg.Broker.ServerUsername,
		Password: cfg.Broker.ServerPassword,
	}, s, bus)
	if err != nil {
		log.Fatalf("create mqtt client: %v", err)
	}
	if err := connectWithRetry(mc); err != nil {
		log.Fatalf("connect mqtt broker: %v", err)
	}
	if err := mc.Subscribe(); err != nil {
		log.Fatalf("subscribe mqtt: %v", err)
	}
	log.Println("MQTT connected and subscribed")

	// 互踢：后端经内部 HTTP 调独立 broker 踢除会话；被踢通知仍由 MQTT 客户端发布。
	kicker := newHTTPSessionKicker(cfg.Sync.BrokerURL, cfg.Sync.Secret)
	handler.SetSessionKicker(kicker)
	mc.SetKicker(kicker)
	handler.SetKickedNotifier(mc)

	h := handler.New(s, mc, bus)
	handler.SeedAdminUsers(s)

	// 在线状态维护（设备级模式，系统默认 connection）：
	//   connection — 上下线由独立 broker 的连接/断开事件实时驱动，扫描不回收；
	//   report     — 周期扫描按设备/全局 offline_timeout_sec 判“超时未上报离线”；
	//   ping       — 周期探活，超时不应答由扫描回收。
	offlineTimeout := time.Duration(cfg.Device.OfflineTimeoutSec) * time.Second
	scanInterval := time.Duration(cfg.Device.ScanIntervalSec) * time.Second
	log.Printf("device online mode: %s (offline_timeout=%s scan=%s; per-device override supported)",
		cfg.Device.OnlineMode, offlineTimeout, scanInterval)
	go func() {
		ticker := time.NewTicker(scanInterval)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.MarkOfflineDevices(cfg.Device.OnlineMode, offlineTimeout); err != nil {
				log.Printf("mark offline devices error: %v", err)
			}
		}
	}()

	// ping 在线判定模式：周期性向有效模式=ping 的启用设备发布探活 wendao/{id}/ping。
	pingInterval := time.Duration(cfg.Device.PingIntervalSec) * time.Second
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			ids, err := s.ListPingModeDevices(cfg.Device.OnlineMode)
			if err != nil {
				log.Printf("list ping-mode devices error: %v", err)
				continue
			}
			for _, id := range ids {
				pingID := fmt.Sprintf("ping-%d", time.Now().UnixNano())
				if err := mc.PublishPing(id, pingID); err != nil {
					log.Printf("publish ping to %s error: %v", id, err)
				}
			}
		}
	}()

	// 控制指令超时回收：每 15s 将超过 30s 未 ack 的下发标记为超时
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if n, err := s.MarkControlTimeout(30 * time.Second); err != nil {
				log.Printf("mark control timeout error: %v", err)
			} else if n > 0 {
				log.Printf("marked %d control commands timeout", n)
			}
		}
	}()

	r := gin.Default()
	r.Use(cors.New(buildCORS(cfg)))

	api := r.Group("/api/v1")
	registerAPIRoutes(api, h, s)

	// WebSocket 实时推送：必须在 auth 组之外（浏览器 WS 握手不能带自定义头）。
	api.GET("/ws", h.ServeWS)

	// broker → 后端的内部通道（仅 compose 内网，不反代公网；X-Sync-Key 鉴权）。
	internal := r.Group("/internal/v1")
	internal.Use(syncKeyAuth(cfg.Sync.Secret))
	internal.POST("/device-status", deviceStatusHandler(s, bus))

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("server start on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server run: %v", err)
	}
}

// connectWithRetry 等待独立 broker 就绪（最长约 2 分钟），broker 已就绪则立即返回。
func connectWithRetry(mc *mqttclient.Client) error {
	const maxAttempts = 60
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := mc.Connect(); err == nil {
			return nil
		} else {
			log.Printf("connect mqtt broker attempt %d/%d failed: %v", attempt, maxAttempts, err)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("mqtt broker not reachable after %d attempts", maxAttempts)
}

// syncKeyAuth 校验 broker → 后端内部请求的共享密钥。
func syncKeyAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader(syncapi.HeaderName) != secret {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

// deviceStatusHandler 接收 broker 上报的设备上下线：落运行态并广播前端。
func deviceStatusHandler(s *store.Store, bus *events.Bus) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req syncapi.DeviceStatusRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.DeviceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
			return
		}
		if err := s.SetDeviceRuntimeStatus(req.DeviceID, req.Online); err != nil {
			log.Printf("device-status: set %s online=%v error: %v", req.DeviceID, req.Online, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "store error"})
			return
		}
		log.Printf("device-status: %s online=%v (client %s)", req.DeviceID, req.Online, req.ClientID)
		if req.TenantID != 0 {
			bus.BroadcastToTenant(req.TenantID, events.Message{
				Type: "device_status",
				Data: map[string]interface{}{"device_id": req.DeviceID, "online": req.Online},
			})
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// httpSessionKicker 经内部 HTTP 调独立 broker 实现互踢，同时满足
// handler.SessionKicker（KickDeviceSessions）与 mqtt.SessionKicker（KickByUsername）。
type httpSessionKicker struct {
	client *syncapi.Client
}

func newHTTPSessionKicker(baseURL, secret string) *httpSessionKicker {
	return &httpSessionKicker{client: syncapi.NewClient(baseURL, secret)}
}

func (k *httpSessionKicker) KickDeviceSessions(deviceID, excludeClientID string) ([]string, error) {
	return k.kick(deviceID, excludeClientID)
}

func (k *httpSessionKicker) KickByUsername(username, excludeClientID string) ([]string, error) {
	return k.kick(username, excludeClientID)
}

func (k *httpSessionKicker) kick(username, excludeClientID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := k.client.Kick(ctx, syncapi.KickRequest{Username: username, ExcludeClientID: excludeClientID})
	if err != nil {
		return nil, err
	}
	return resp.Kicked, nil
}

func registerAPIRoutes(api *gin.RouterGroup, h *handler.Handler, s *store.Store) {
	api.GET("/health", h.HealthCheck)
	api.POST("/login", h.Login)

	auth := api.Group("", handler.AuthMiddleware(s))
	{
		// 当前登录账号的界面偏好（每账号各自保存，/me/preferences）
		auth.GET("/me/preferences", h.GetMyPreferences)
		auth.PUT("/me/preferences", h.UpdateMyPreferences)

		auth.GET("/dashboard/stats", h.GetDashboardStats)
		auth.GET("/dashboard/traffic", h.GetDashboardTraffic)
		auth.GET("/projects/:id/data", h.GetProjectData)

		// 租户管理：仅超管（路由级强制）
		auth.POST("/tenants", handler.RequireRole(model.RoleSuperAdmin), h.CreateTenant)
		auth.GET("/tenants", handler.RequireRole(model.RoleSuperAdmin), h.ListTenants)
		auth.PUT("/tenants/:id", handler.RequireRole(model.RoleSuperAdmin), h.UpdateTenant)
		auth.DELETE("/tenants/:id", handler.RequireRole(model.RoleSuperAdmin), h.DeleteTenant)

		auth.POST("/projects", h.CreateProject)
		auth.GET("/projects", h.ListProjects)
		auth.GET("/projects/device-stats", h.GetProjectDeviceStats)
		auth.PUT("/projects/:id", h.UpdateProject)
		auth.PUT("/projects/:id/settings", h.UpdateProjectSettings)
		auth.POST("/projects/:id/apply-online-default", h.ApplyProjectOnlineDefault)
		auth.DELETE("/projects/:id", h.DeleteProject)

		auth.POST("/products", h.CreateProduct)
		auth.GET("/products", h.ListProducts)
		auth.PUT("/products/:key", h.UpdateProduct)
		auth.DELETE("/products/:key", h.DeleteProduct)
		auth.POST("/products/:key/secret/reset", h.ResetProductSecret)

		auth.POST("/devices/batch-preregister", h.BatchPreregister)
		auth.POST("/devices", h.CreateDevice)
		auth.GET("/devices", h.ListDevices)
		auth.GET("/devices/page", h.ListDevicesPage)
		auth.GET("/devices/:deviceId", h.GetDevice)
		auth.PUT("/devices/:deviceId", h.UpdateDevice)
		auth.DELETE("/devices/:deviceId", h.DeleteDevice)
		auth.PUT("/devices/:deviceId/enabled", h.SetDeviceEnabled)
		auth.POST("/devices/:deviceId/secret/reset", h.ResetDeviceSecret)
		auth.POST("/devices/:deviceId/reactivate", h.ReactivateDevice)

		auth.POST("/devices/:deviceId/tags", h.CreateDeviceTag)
		auth.GET("/devices/:deviceId/tags", h.ListDeviceTags)
		auth.DELETE("/devices/:deviceId/tags", h.DeleteDeviceTag)

		auth.POST("/projects/:id/tags", h.CreateProjectTag)
		auth.GET("/projects/:id/tags", h.ListProjectTags)
		auth.DELETE("/projects/:id/tags", h.DeleteProjectTag)

		auth.GET("/devices/:deviceId/data", h.GetDeviceData)
		auth.DELETE("/devices/:deviceId/data", handler.RequireRole(model.RoleSuperAdmin), h.DeleteDeviceData)
		auth.POST("/devices/:deviceId/control", h.SendControl)
		auth.GET("/devices/:deviceId/control/:msgId", h.GetControlStatus)

		auth.GET("/projects/:id/commands", h.ListControlCommands)
		auth.POST("/projects/:id/commands", h.CreateControlCommand)
		auth.PUT("/commands/:id", h.UpdateControlCommand)
		auth.DELETE("/commands/:id", h.DeleteControlCommand)

		auth.GET("/control-logs", h.ListControlLogs)
		auth.DELETE("/control-logs", handler.RequireRole(model.RoleSuperAdmin), h.DeleteControlLogs)

		auth.GET("/devices/:deviceId/peer/messages", h.ListPeerMessages)
		auth.GET("/peer/allows", h.ListPeerAllows)
		auth.POST("/peer/allows", h.CreatePeerAllow)
		auth.DELETE("/peer/allows/:id", h.DeletePeerAllow)

		auth.POST("/firmwares", h.CreateFirmware)
		auth.GET("/firmwares", h.ListFirmwares)
		auth.DELETE("/firmwares", h.DeleteFirmware)
		auth.GET("/firmwares/latest", h.GetDeviceFirmware)

		auth.POST("/ota/tasks", h.CreateOTATask)
		auth.GET("/ota/tasks", h.ListOTATasks)
		auth.GET("/ota/logs", h.GetOTALogs)
		auth.DELETE("/ota/logs", handler.RequireRole(model.RoleSuperAdmin), h.DeleteOTALogs)

		auth.GET("/users", h.ListUsers)
		auth.PUT("/users/password", h.ChangePassword)
		auth.PUT("/users/:id/password", h.AdminResetUserPassword)
		auth.DELETE("/users/:id", h.DeleteUser)
	}
}

func buildCORS(cfg *config.Config) cors.Config {
	origins := cfg.Server.CorsOrigins
	if len(origins) == 0 {
		// 未配置则放行本机开发来源；生产应通过 config/env 明确白名单。
		origins = []string{
			"http://localhost:3000", "http://127.0.0.1:3000",
			"http://localhost:3001", "http://127.0.0.1:3001",
		}
	}
	c := cors.DefaultConfig()
	c.AllowOrigins = origins
	c.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	c.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	c.ExposeHeaders = []string{"Content-Length", "Content-Disposition"}
	c.AllowCredentials = true
	c.MaxAge = 12 * time.Hour
	return c
}
