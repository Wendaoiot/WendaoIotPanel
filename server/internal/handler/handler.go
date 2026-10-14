package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wendaoiotpanel/internal/events"
	"wendaoiotpanel/internal/metrics"
	"wendaoiotpanel/internal/model"
	"wendaoiotpanel/internal/protocol"
	"wendaoiotpanel/internal/store"
	cryptopkg "wendaoiotpanel/pkg/crypto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// deviceIDPattern 设备 ID 规范：1-64 位字母/数字/下划线/短横线（大小写敏感）。
var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Handler struct {
	store *store.Store
	mqtt  MQTTPublisher
	bus   *events.Bus
}

type MQTTPublisher interface {
	PublishControl(deviceID string, cmd *protocol.DownlinkRequest) error
	PublishRaw(topic string, payload []byte) error
}

func New(s *store.Store, mqtt MQTTPublisher, bus *events.Bus) *Handler {
	return &Handler{store: s, mqtt: mqtt, bus: bus}
}

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

func success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 0, Msg: "ok", Data: data})
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{Code: code, Msg: msg})
}

func forbidden(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusForbidden, Response{Code: 403, Msg: msg})
}

func (h *Handler) HealthCheck(c *gin.Context) {
	success(c, gin.H{"status": "ok", "time": time.Now().Unix()})
}

func getAuthInfo(c *gin.Context) (role string, tenantID *uint) {
	role = c.GetString("role")
	val, exists := c.Get("tenant_id")
	if exists && val != nil {
		if tid, ok := val.(*uint); ok {
			tenantID = tid
		}
	}
	return
}

func (h *Handler) assertProjectBelongsToTenant(projectID uint, tenantID *uint) bool {
	if tenantID == nil {
		return true
	}
	p, err := h.store.GetProjectByID(projectID)
	if err != nil {
		return false
	}
	return p.TenantID == *tenantID
}

// Tenant

func (h *Handler) CreateTenant(c *gin.Context) {
	if role, _ := getAuthInfo(c); role != model.RoleSuperAdmin {
		forbidden(c, "仅平台超级管理员可创建租户")
		return
	}
	var req struct {
		Name      string `json:"name" binding:"required"`
		AdminUser string `json:"admin_username"` // 可选：指定管理员登录名；不填则用 <租户名>_admin
		AdminPwd  string `json:"admin_pwd"`      // 可选：不填则生成随机一次性密码
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}
	t := &model.Tenant{Name: req.Name}
	if exists, err := h.store.TenantNameExists(req.Name); err == nil && exists {
		fail(c, http.StatusBadRequest, "租户名称已存在")
		return
	}
	if err := h.store.CreateTenant(t); err != nil {
		if strings.Contains(err.Error(), "1062") || strings.Contains(err.Error(), "Duplicate") {
			fail(c, http.StatusBadRequest, "租户名称已存在")
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	pwd := req.AdminPwd
	generated := false
	if pwd == "" {
		p, err := cryptopkg.RandomPassword(12)
		if err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		pwd = p
		generated = true
	}

	hash := cryptopkg.MustHashPassword(pwd)
	usernames := []string{}
	if req.AdminUser != "" {
		usernames = append(usernames, req.AdminUser)
	}
	usernames = append(usernames, t.Name+"_admin", fmt.Sprintf("tenant_%d", t.ID))

	var created *model.AdminUser
	for _, uname := range usernames {
		if uname == "" {
			continue
		}
		u := &model.AdminUser{Username: uname, Password: hash, Role: model.RoleTenantAdmin, TenantID: &t.ID}
		if err := h.store.CreateAdminUser(u); err == nil {
			created = u
			break
		}
	}
	if created == nil {
		fail(c, http.StatusConflict, "租户已创建，但管理员账号因用户名冲突未生成，请在用户管理中手动创建")
		return
	}

	success(c, gin.H{
		"tenant":     t,
		"admin_user": created.Username,
		"admin_pwd":  pwd,
		"generated":  generated,
	})
}

func (h *Handler) ListTenants(c *gin.Context) {
	if role, _ := getAuthInfo(c); role != model.RoleSuperAdmin {
		forbidden(c, "仅平台超级管理员可查看租户列表")
		return
	}
	tenants, err := h.store.ListTenants()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, tenants)
}

func (h *Handler) UpdateTenant(c *gin.Context) {
	if role, _ := getAuthInfo(c); role != model.RoleSuperAdmin {
		forbidden(c, "仅平台超级管理员可修改租户")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if exists, err := h.store.TenantNameExists(req.Name); err == nil && exists {
		// 自己改名（占用行是自身）时放行
		if t, gerr := h.store.GetTenantByID(uint(id)); gerr == nil && t.Name == req.Name {
			success(c, nil)
			return
		}
		fail(c, http.StatusBadRequest, "租户名称已存在")
		return
	}
	if err := h.store.UpdateTenant(uint(id), req.Name); err != nil {
		if strings.Contains(err.Error(), "1062") || strings.Contains(err.Error(), "Duplicate") {
			fail(c, http.StatusBadRequest, "租户名称已存在")
			return
		}
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

func (h *Handler) DeleteTenant(c *gin.Context) {
	if role, _ := getAuthInfo(c); role != model.RoleSuperAdmin {
		forbidden(c, "仅平台超级管理员可删除租户")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.DeleteTenant(uint(id)); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// Project

func (h *Handler) CreateProject(c *gin.Context) {
	role, tenantID := getAuthInfo(c)
	var req struct {
		TenantID uint   `json:"tenant_id"`
		Name     string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}
	if role == model.RoleTenantAdmin {
		if tenantID == nil {
			fail(c, -1, "租户信息异常")
			return
		}
		req.TenantID = *tenantID
	}
	if req.TenantID == 0 {
		fail(c, -1, "请指定所属租户")
		return
	}
	p := &model.Project{TenantID: req.TenantID, Name: req.Name}
	if err := h.store.CreateProject(p); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, p)
}

func (h *Handler) ListProjects(c *gin.Context) {
	role, tenantID := getAuthInfo(c)
	if role == model.RoleTenantAdmin && tenantID == nil {
		fail(c, -1, "租户信息异常")
		return
	}

	if tenantID != nil {
		projects, err := h.store.ListProjectsByTenant(*tenantID)
		if err != nil {
			fail(c, -1, err.Error())
			return
		}
		success(c, projects)
		return
	}

	var req struct {
		TenantID uint `form:"tenant_id"`
	}
	c.ShouldBindQuery(&req)

	var projects []model.Project
	var err error
	if req.TenantID > 0 {
		projects, err = h.store.ListProjectsByTenant(req.TenantID)
	} else {
		projects, err = h.store.ListAllProjects()
	}
	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, projects)
}

func (h *Handler) UpdateProject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, -1, "invalid id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(id), tenantID) {
		fail(c, 403, "无权操作此项目")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}
	if err := h.store.UpdateProject(uint(id), req.Name); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, nil)
}

func (h *Handler) DeleteProject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, -1, "invalid id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(id), tenantID) {
		fail(c, 403, "无权操作此项目")
		return
	}
	if err := h.store.DeleteProject(uint(id)); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, nil)
}

// UpdateProjectSettings 保存项目级在线判定默认（空串/0 = 沿用系统默认）。
func (h *Handler) UpdateProjectSettings(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, -1, "invalid id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(id), tenantID) {
		fail(c, 403, "无权操作此项目")
		return
	}
	mode, timeoutSec, ok := parseOnlineDefaultBody(c)
	if !ok {
		return
	}
	if err := h.store.UpdateProjectSettings(uint(id), mode, timeoutSec); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// ApplyProjectOnlineDefault 把项目在线判定默认显式写入该项目全部设备（覆盖设备各自设置）。
func (h *Handler) ApplyProjectOnlineDefault(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, -1, "invalid id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(id), tenantID) {
		fail(c, 403, "无权操作此项目")
		return
	}
	mode, timeoutSec, ok := parseOnlineDefaultBody(c)
	if !ok {
		return
	}
	// 一键应用要求显式模式：不能把“跟随系统默认”的空值固化到每台设备上
	if mode == "" {
		fail(c, http.StatusBadRequest, "请先把项目默认设为具体的判定方式（仅按连接/按上报时间/按应答信号）再应用")
		return
	}
	n, err := h.store.ApplyProjectOnlineDefault(uint(id), mode, timeoutSec)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"affected": n})
}

// parseOnlineDefaultBody 解析并校验在线判定默认请求体。
// 合法返回 (归一化模式, 超时秒, true)；非法已写响应，返回 false。
func parseOnlineDefaultBody(c *gin.Context) (string, int, bool) {
	var req struct {
		OnlineMode        string `json:"online_mode"`
		OfflineTimeoutSec *int   `json:"offline_timeout_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return "", 0, false
	}
	mode := strings.ToLower(strings.TrimSpace(req.OnlineMode))
	switch mode {
	case "", "connection", "report", "ping":
	default:
		fail(c, http.StatusBadRequest, "online_mode 仅支持：connection/report/ping（或空串表示沿用系统默认）")
		return "", 0, false
	}
	timeoutSec := 0
	if req.OfflineTimeoutSec != nil {
		if *req.OfflineTimeoutSec < 0 || *req.OfflineTimeoutSec > 604800 {
			fail(c, http.StatusBadRequest, "offline_timeout_sec 取值范围 0(沿用系统时限)~604800")
			return "", 0, false
		}
		timeoutSec = *req.OfflineTimeoutSec
	}
	return mode, timeoutSec, true
}

// Device

func (h *Handler) CreateDevice(c *gin.Context) {
	_, tenantID := getAuthInfo(c)
	var req struct {
		ID        string `json:"id" binding:"required"`
		ProjectID uint   `json:"project_id" binding:"required"`
		Name      string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}
	// 设备 ID 规范：1-64 位字母/数字/下划线/短横线；大小写敏感（utf8mb4_bin）
	if !deviceIDPattern.MatchString(req.ID) {
		fail(c, http.StatusBadRequest, "设备ID不合法：仅允许 1-64 位字母/数字/下划线/短横线，且区分大小写")
		return
	}
	if !h.assertProjectBelongsToTenant(req.ProjectID, tenantID) {
		fail(c, 403, "无权在此项目中创建设备")
		return
	}
	proj, err := h.store.GetProjectByID(req.ProjectID)
	if err != nil {
		fail(c, http.StatusBadRequest, "项目不存在")
		return
	}
	secret, err := cryptopkg.RandomPassword(20)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	secretHash := cryptopkg.MustHashPassword(secret)
	d := &model.Device{
		ID:           req.ID,
		ProjectID:    req.ProjectID,
		TenantID:     proj.TenantID,
		Name:         req.Name,
		Status:       model.DeviceStatusOffline,
		Enabled:      true,
		DeviceSecret: secretHash,
	}
	if err := h.store.CreateDeviceWithSecret(d, secretHash); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	success(c, gin.H{"device": d, "device_secret": secret, "secret_note": "接入密钥仅此一次返回，请妥善保存"})
}

func (h *Handler) ListDevices(c *gin.Context) {
	_, tenantID := getAuthInfo(c)
	var req struct {
		ProjectID *uint `form:"project_id"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}

	var devices []model.Device
	if req.ProjectID != nil {
		if !h.assertProjectBelongsToTenant(*req.ProjectID, tenantID) {
			fail(c, 403, "无权查看此项目的设备")
			return
		}
		devices, _ = h.store.ListDevicesByProject(*req.ProjectID)
	} else if tenantID != nil {
		// 未指定项目时，列出当前租户所有项目的设备
		projects, err := h.store.ListProjectsByTenant(*tenantID)
		if err != nil {
			fail(c, -1, err.Error())
			return
		}
		var projectIDs []uint
		for _, p := range projects {
			projectIDs = append(projectIDs, p.ID)
		}
		if len(projectIDs) == 0 {
			success(c, []model.Device{})
			return
		}
		devices, err = h.store.ListDevicesByProjects(projectIDs)
		if err != nil {
			fail(c, -1, err.Error())
			return
		}
	} else {
		// 超级管理员未指定项目，列出所有设备
		devices, _ = h.store.ListDevicesByProjects(nil)
	}

	success(c, devices)
}

// ListDevicesPage GET /devices/page
// 分页检索设备（千台规模）：project_id / keyword(SN或名称) / status / page / page_size。
// 旧 GET /devices 全量语义保留给下拉等场景；本接口供设备管理卡片列表使用。
func (h *Handler) ListDevicesPage(c *gin.Context) {
	_, tenantID := getAuthInfo(c)

	var q struct {
		ProjectID uint   `form:"project_id"`
		Keyword   string `form:"keyword"`
		Status    string `form:"status"`
		Page      int    `form:"page"`
		PageSize  int    `form:"page_size"`
	}
	if err := c.ShouldBindQuery(&q); err != nil {
		fail(c, -1, err.Error())
		return
	}

	switch strings.TrimSpace(q.Status) {
	case "", "online", "offline", "disabled", "pending":
	default:
		fail(c, http.StatusBadRequest, "status 仅支持：online/offline/disabled/pending")
		return
	}

	var scopeProjectIDs []uint
	if tenantID != nil {
		projects, err := h.store.ListProjectsByTenant(*tenantID)
		if err != nil {
			fail(c, -1, err.Error())
			return
		}
		for _, p := range projects {
			scopeProjectIDs = append(scopeProjectIDs, p.ID)
		}
		if len(scopeProjectIDs) == 0 {
			success(c, gin.H{"total": 0, "page": 1, "page_size": 24, "items": []model.Device{}})
			return
		}
	}

	// 指定项目时仍做租户归属校验，防止越权遍历他人项目。
	if q.ProjectID > 0 && !h.assertProjectBelongsToTenant(q.ProjectID, tenantID) {
		fail(c, 403, "无权查看此项目的设备")
		return
	}

	page := q.Page
	if page <= 0 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = 24
	}
	if pageSize > 100 {
		pageSize = 100
	}

	result, err := h.store.ListDevicesPage(store.DevicePageFilter{
		ProjectIDs: scopeProjectIDs,
		ProjectID:  q.ProjectID,
		Keyword:    q.Keyword,
		Status:     strings.TrimSpace(q.Status),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, result)
}

// GetProjectDeviceStats GET /projects/device-stats
// 返回各项目设备计数（文件夹卡片角标）：{project_id:{total,online,offline,pending,disabled}}。
// 超管可 ?tenant_id= 限定租户；租户管理员强制本租户。无设备/不可见项目由前端补零。
func (h *Handler) GetProjectDeviceStats(c *gin.Context) {
	role, tenantID := getAuthInfo(c)
	if role == model.RoleTenantAdmin && tenantID == nil {
		fail(c, -1, "租户信息异常")
		return
	}

	var scopeTenant *uint
	if tenantID != nil {
		scopeTenant = tenantID
	} else {
		// 超级管理员：可按 tenant_id 过滤，缺省=全部
		var req struct {
			TenantID uint `form:"tenant_id"`
		}
		c.ShouldBindQuery(&req)
		if req.TenantID > 0 {
			t := req.TenantID
			scopeTenant = &t
		}
	}

	var projectIDs []uint
	if scopeTenant != nil {
		projects, err := h.store.ListProjectsByTenant(*scopeTenant)
		if err != nil {
			fail(c, -1, err.Error())
			return
		}
		for _, p := range projects {
			projectIDs = append(projectIDs, p.ID)
		}
	}

	rows, err := h.store.AggregateDeviceCountsByProjects(projectIDs)
	if err != nil {
		fail(c, -1, err.Error())
		return
	}

	out := gin.H{}
	for _, r := range rows {
		offline := r.Total - r.Online - r.Disabled - r.Pending
		if offline < 0 {
			offline = 0
		}
		out[strconv.FormatUint(uint64(r.ProjectID), 10)] = gin.H{
			"total":    r.Total,
			"online":   r.Online,
			"offline":  offline,
			"pending":  r.Pending,
			"disabled": r.Disabled,
		}
	}
	success(c, out)
}

func (h *Handler) GetDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		fail(c, 403, "无权查看此设备")
		return
	}
	device, err := h.store.GetDevice(deviceID)
	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, device)
}

func (h *Handler) UpdateDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		fail(c, 403, "无权操作此设备")
		return
	}
	var req struct {
		Name      string `json:"name" binding:"required"`
		ProjectID uint   `json:"project_id" binding:"required"`
		// 设备级在线判定：''=跟随项目默认（项目也为空则跟系统默认，最终 connection）、
		// connection/report/ping=设备显式覆盖。超时 0=沿用上级时限，上限 7 天
		OnlineMode        string `json:"online_mode"`
		OfflineTimeoutSec *int   `json:"offline_timeout_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.OnlineMode)) {
	case "", "connection", "report", "ping":
		req.OnlineMode = strings.ToLower(strings.TrimSpace(req.OnlineMode))
	default:
		fail(c, -1, "online_mode 仅支持：connection/report/ping（或空串表示跟随项目默认）")
		return
	}
	offlineTimeoutSec := 0
	if req.OfflineTimeoutSec != nil {
		if *req.OfflineTimeoutSec < 0 || *req.OfflineTimeoutSec > 604800 {
			fail(c, -1, "offline_timeout_sec 取值范围 0(沿用上级时限)~604800")
			return
		}
		offlineTimeoutSec = *req.OfflineTimeoutSec
	}
	if !h.assertProjectBelongsToTenant(req.ProjectID, tenantID) {
		fail(c, 403, "无权将设备迁移到此项目")
		return
	}
	if err := h.store.UpdateDevice(deviceID, req.Name, req.ProjectID, req.OnlineMode, offlineTimeoutSec); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, nil)
}

func (h *Handler) DeleteDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		fail(c, 403, "无权操作此设备")
		return
	}
	if err := h.store.DeleteDevice(deviceID); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, nil)
}

// DeviceTag

func (h *Handler) CreateDeviceTag(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		forbidden(c, "无权操作此设备")
		return
	}
	var req struct {
		TagKey    string `json:"tag_key" binding:"required"`
		Name      string `json:"name"`
		Unit      string `json:"unit"`
		Interface string `json:"interface"`
		Formula   string `json:"formula"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	t := &model.DeviceTag{
		DeviceID:  deviceID,
		TagKey:    req.TagKey,
		Name:      req.Name,
		Unit:      req.Unit,
		Interface: req.Interface,
		Formula:   req.Formula,
	}
	if err := h.store.CreateDeviceTag(t); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, t)
}

func (h *Handler) ListDeviceTags(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		forbidden(c, "无权查看此设备")
		return
	}
	tags, err := h.store.ListDeviceTags(deviceID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, tags)
}

func (h *Handler) DeleteDeviceTag(c *gin.Context) {
	var req struct {
		ID uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	_, tenantID := getAuthInfo(c)
	tag, err := h.store.GetDeviceTagByID(req.ID)
	if err != nil {
		fail(c, http.StatusNotFound, "标签不存在")
		return
	}
	if !h.assertDeviceBelongsToTenant(tag.DeviceID, tenantID) {
		forbidden(c, "无权操作此设备标签")
		return
	}
	if err := h.store.DeleteDeviceTag(req.ID); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// ProjectTag

func (h *Handler) CreateProjectTag(c *gin.Context) {
	projectID := c.Param("id")
	pid, err := strconv.ParseUint(projectID, 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid project id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(pid), tenantID) {
		forbidden(c, "无权操作此项目")
		return
	}
	var req struct {
		TagKey   string `json:"tag_key" binding:"required"`
		TagName  string `json:"tag_name"`
		Unit     string `json:"unit"`
		DataType string `json:"data_type"`
		Writable bool   `json:"writable"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.TagName == "" {
		req.TagName = req.TagKey
	}
	if req.DataType == "" {
		req.DataType = "number"
	}
	t := &model.ProjectTag{
		ProjectID: uint(pid),
		TagKey:    req.TagKey,
		TagName:   req.TagName,
		Unit:      req.Unit,
		DataType:  req.DataType,
		Writable:  req.Writable,
	}
	if err := h.store.CreateProjectTag(t); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, t)
}

func (h *Handler) ListProjectTags(c *gin.Context) {
	projectID := c.Param("id")
	pid, err := strconv.ParseUint(projectID, 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid project id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(pid), tenantID) {
		forbidden(c, "无权查看此项目")
		return
	}
	tags, err := h.store.ListProjectTags(uint(pid))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, tags)
}

func (h *Handler) DeleteProjectTag(c *gin.Context) {
	var req struct {
		ID uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	_, tenantID := getAuthInfo(c)
	tag, err := h.store.GetProjectTagByID(req.ID)
	if err != nil {
		fail(c, http.StatusNotFound, "标签不存在")
		return
	}
	if !h.assertProjectBelongsToTenant(tag.ProjectID, tenantID) {
		forbidden(c, "无权操作此项目标签")
		return
	}
	if err := h.store.DeleteProjectTag(req.ID); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// DeviceData

func (h *Handler) GetDeviceData(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		forbidden(c, "无权查看此设备数据")
		return
	}
	limit := 100
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			offset = v
		}
	}
	if limit > 500 {
		limit = 500
	}
	var start, end *time.Time
	if t, err := parseTimeQuery(c.Query("start")); err == nil && t != nil {
		start = t
	}
	if t, err := parseTimeQuery(c.Query("end")); err == nil && t != nil {
		end = t
	}
	data, total, err := h.store.ListDeviceDataRange(deviceID, start, end, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	type dataItem struct {
		ID        uint                   `json:"id"`
		DeviceID  string                 `json:"device_id"`
		MsgID     string                 `json:"msg_id"`
		Ts        int64                  `json:"ts"`
		DeviceTs  int64                  `json:"device_ts"`
		Version   string                 `json:"version"`
		Data      map[string]interface{} `json:"data"`
		CreatedAt interface{}            `json:"created_at"`
	}
	result := make([]dataItem, 0, len(data))
	for _, d := range data {
		var parsed map[string]interface{}
		json.Unmarshal([]byte(d.Data), &parsed)
		result = append(result, dataItem{
			ID:        d.ID,
			DeviceID:  d.DeviceID,
			MsgID:     d.MsgID,
			Ts:        d.Ts,
			DeviceTs:  d.DeviceTs,
			Version:   d.Version,
			Data:      parsed,
			CreatedAt: d.CreatedAt,
		})
	}
	if strings.EqualFold(c.Query("export"), "csv") {
		h.exportDeviceDataCSV(c, data)
		return
	}
	success(c, gin.H{
		"list":   result,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Control

func (h *Handler) SendControl(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		forbidden(c, "无权操作此设备")
		return
	}
	var req struct {
		Tags map[string]float64 `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		fail(c, http.StatusNotFound, "设备不存在")
		return
	}
	if !dev.Enabled {
		fail(c, http.StatusConflict, "设备已被禁用，无法下发指令")
		return
	}
	if dev.Status != model.DeviceStatusOnline {
		fail(c, http.StatusConflict, "设备当前离线，指令无法送达；请等待设备上线后再操作")
		return
	}

	msgID := uuid.New().String()
	cmd := &protocol.DownlinkRequest{
		ID:   msgID,
		Ts:   time.Now().UnixMilli(),
		Tags: req.Tags,
	}

	tagsJSON, _ := json.Marshal(req.Tags)
	clog := &model.ControlLog{
		DeviceID: deviceID,
		TenantID: dev.TenantID,
		MsgID:    msgID,
		Tags:     string(tagsJSON),
		Status:   model.ControlStatusPending,
	}
	if err := h.store.CreateControlLogScoped(clog); err != nil {
		log.Printf("create control log error: %v", err)
	}

	if err := h.mqtt.PublishControl(deviceID, cmd); err != nil {
		_ = h.store.ApplyControlAck(msgID, -1, "下发失败: "+err.Error())
		fail(c, -1, err.Error())
		return
	}
	_ = h.store.SetControlLogDeliveredOrErr(msgID, true)
	// 仪表盘消息流出实时计数（业务指令；OTA/ping 不计）
	metrics.AddOut(dev.TenantID)

	success(c, gin.H{
		"msg_id": msgID,
		"status": model.ControlStatusDelivered,
		"hint":   "指令已下发，设备执行结果将在数秒内回传（可在本页/控制日志查看 ack）",
	})
}

func (h *Handler) ListControlLogs(c *gin.Context) {
	deviceID := c.Query("device_id")
	if deviceID != "" {
		_, tenantID := getAuthInfo(c)
		if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
			forbidden(c, "无权查看该设备的控制日志")
			return
		}
	}
	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			offset = v
		}
	}
	var start, end *time.Time
	if t, err := parseTimeQuery(c.Query("start")); err == nil && t != nil {
		start = t
	}
	if t, err := parseTimeQuery(c.Query("end")); err == nil && t != nil {
		end = t
	}
	_, tenantID := getAuthInfo(c)
	logs, total, err := h.store.ListControlLogsScoped(tenantID, deviceID, start, end, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 支持导出 CSV
	if strings.EqualFold(c.Query("export"), "csv") {
		h.exportControlLogsCSV(c, logs)
		return
	}
	success(c, gin.H{"list": logs, "total": total})
}

// Dashboard

func (h *Handler) GetDashboardStats(c *gin.Context) {
	_, tenantID := getAuthInfo(c)
	stats, err := h.store.GetDashboardStats(tenantID)
	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	// 实时消息速率（当前服务节点内存环形桶，重启清零；历史量见数据库计数字段）
	snap := metrics.SnapshotPoints(tenantID, 30)
	success(c, gin.H{
		"total_tenants":         stats.TotalTenants,
		"total_projects":        stats.TotalProjects,
		"total_devices":         stats.TotalDevices,
		"online_devices":        stats.OnlineDevices,
		"disabled_devices":      stats.DisabledDevices,
		"pending_devices":       stats.PendingDevices,
		"messages_in_24h":       stats.MessagesIn24H,
		"messages_out_24h":      stats.MessagesOut24H,
		"messages_in_total_db":  stats.MessagesInAll,
		"messages_out_total_db": stats.MessagesOutAll,
		"in_rate":               snap.InRate,
		"out_rate":              snap.OutRate,
		"in_total":              snap.InTotal,
		"out_total":             snap.OutTotal,
		"series_in":             snap.SeriesIn,
		"series_out":            snap.SeriesOut,
	})
}

// ProjectData 项目数据聚合
func (h *Handler) GetProjectData(c *gin.Context) {
	projectID := c.Param("id")
	pid, err := strconv.ParseUint(projectID, 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid project id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(pid), tenantID) {
		forbidden(c, "无权查看此项目数据")
		return
	}

	projTags, err := h.store.ListProjectTags(uint(pid))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	devices, err := h.store.ListDevicesByProject(uint(pid))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	deviceTagMap := make(map[string]map[string]model.DeviceTag)
	for _, d := range devices {
		tags, _ := h.store.ListDeviceTags(d.ID)
		tm := make(map[string]model.DeviceTag)
		for _, t := range tags {
			tm[t.TagKey] = t
		}
		deviceTagMap[d.ID] = tm
	}

	type DeviceValue struct {
		DeviceID   string  `json:"device_id"`
		DeviceName string  `json:"device_name"`
		Value      float64 `json:"value"`
		Ts         int64   `json:"ts"`
		Status     int     `json:"status"`
	}

	type TagData struct {
		TagKey   string        `json:"tag_key"`
		TagName  string        `json:"tag_name"`
		Unit     string        `json:"unit"`
		DataType string        `json:"data_type"`
		Writable bool          `json:"writable"`
		Devices  []DeviceValue `json:"devices"`
	}

	result := make([]TagData, 0, len(projTags))
	for _, pt := range projTags {
		td := TagData{
			TagKey:   pt.TagKey,
			TagName:  pt.TagName,
			Unit:     pt.Unit,
			DataType: pt.DataType,
			Writable: pt.Writable,
			Devices:  make([]DeviceValue, 0),
		}

		for _, d := range devices {
			// 默认映射：项目字典中的数据点对项目下所有设备生效，无需再为每台设备
			// 单独配置同键设备标签；设备标签仅用于需要接口/公式等设备级覆盖的场景。
			dv := DeviceValue{
				DeviceID:   d.ID,
				DeviceName: d.Name,
				Status:     d.Status,
			}
			latest, err := h.store.GetLatestDataForDevice(d.ID)
			if err == nil {
				var parsed map[string]float64
				json.Unmarshal([]byte(latest.Data), &parsed)
				if v, ok := parsed[pt.TagKey]; ok {
					dv.Value = v
				}
				dv.Ts = latest.Ts
			}
			td.Devices = append(td.Devices, dv)
		}
		result = append(result, td)
	}

	success(c, gin.H{
		"project_id": pid,
		"tags":       result,
	})
}
