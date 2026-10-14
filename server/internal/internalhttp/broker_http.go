// Package internalhttp 提供 broker 进程的内网 HTTP 端点：
//   - GET  /internal/health                       存活探针（compose healthcheck）
//   - POST /internal/v1/kick                      后端请求按 username 踢除会话
// 以及 broker → 后端的设备状态上报客户端（StatusSink）。
// 全部端点仅监听 compose 内网，不映射宿主，且以 X-Sync-Key 共享密钥鉴权。
package internalhttp

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"wendaoiotpanel/internal/syncapi"
)

// Kicker 由 *broker.Broker 满足。
type Kicker interface {
	KickByUsername(username, excludeClientID string) ([]string, error)
}

// Server 承载 broker 侧内部 HTTP。
type Server struct {
	addr   string
	secret string
	kick   Kicker
	http   *http.Server
}

// NewServer 创建 broker 内部 HTTP 服务。
func NewServer(addr, secret string, kick Kicker) *Server {
	s := &Server{addr: addr, secret: secret, kick: kick}

	mux := http.NewServeMux()
	mux.HandleFunc(syncapi.PathKick, s.requireSecret(s.handleKick))
	mux.HandleFunc("/internal/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	s.http = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Start 在后台 goroutine 启动监听。
func (s *Server) Start() {
	go func() {
		log.Printf("broker internal http listening on %s", s.addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("broker internal http exited: %v", err)
		}
	}()
}

// Close 优雅关闭。
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.http.Shutdown(ctx)
}

func (s *Server) requireSecret(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(syncapi.HeaderName) != s.secret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleKick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req syncapi.KickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	kicked, err := s.kick.KickByUsername(req.Username, req.ExcludeClientID)
	if err != nil {
		http.Error(w, "kick failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(syncapi.KickResponse{Kicked: kicked})
}

// StatusSink 实现 broker.StatusSink：把设备上下线异步上报给后端，绝不阻塞 MQTT 事件循环。
// 后端短暂不可用时队列缓冲，满则丢弃并告警（后端离线扫描兜底回收状态）。
type StatusSink struct {
	client *syncapi.Client
	queue  chan syncapi.DeviceStatusRequest
}

// NewStatusSink 构造设备状态上报器并启动后台 worker。
func NewStatusSink(serverURL, secret string) *StatusSink {
	s := &StatusSink{
		client: syncapi.NewClient(serverURL, secret),
		queue:  make(chan syncapi.DeviceStatusRequest, 4096),
	}
	go s.worker()
	return s
}

// SetDeviceRuntime 将一次上下线事件入队（非阻塞）。
func (s *StatusSink) SetDeviceRuntime(deviceID string, online bool, clientID string, tenantID uint) {
	ev := syncapi.DeviceStatusRequest{
		DeviceID: deviceID,
		Online:   online,
		ClientID: clientID,
		TenantID: tenantID,
	}
	select {
	case s.queue <- ev:
	default:
		log.Printf("broker status sink: queue full, drop event device=%s online=%v", deviceID, online)
	}
}

// worker 串行重试上报，后端短暂中断时本地排队，恢复后补发。
func (s *StatusSink) worker() {
	for ev := range s.queue {
		s.sendWithRetry(ev)
	}
}

func (s *StatusSink) sendWithRetry(ev syncapi.DeviceStatusRequest) {
	const maxAttempts = 20
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := s.client.ReportDeviceStatus(ctx, ev)
		cancel()
		if err == nil {
			return
		}
		log.Printf("broker status sink: report device=%s online=%v attempt=%d failed: %v",
			ev.DeviceID, ev.Online, attempt, err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	log.Printf("broker status sink: give up device=%s online=%v after %d attempts",
		ev.DeviceID, ev.Online, maxAttempts)
}
