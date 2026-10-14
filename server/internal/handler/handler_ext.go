package handler

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wendaoiotpanel/internal/events"
	"wendaoiotpanel/internal/model"
	cryptopkg "wendaoiotpanel/pkg/crypto"

	"github.com/gin-gonic/gin"
)

// isHTTPSURL 校验固件下载地址必须为 https（防止 OTA 明文篡改）。
func isHTTPSURL(u string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(u)), "https://")
}

// exportDeviceDataCSV 导出设备遥测数据为 CSV（权威时间戳 ts 为毫秒）。
func (h *Handler) exportDeviceDataCSV(c *gin.Context, data []model.DeviceData) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=device_data.csv")
	w := csv.NewWriter(c.Writer)
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	_ = w.Write([]string{"ID", "设备ID", "消息ID", "时间(ms)", "设备时间(ms)", "版本", "数据(JSON)", "入库时间"})
	for _, d := range data {
		_ = w.Write([]string{
			strconv.FormatUint(uint64(d.ID), 10),
			d.DeviceID,
			d.MsgID,
			strconv.FormatInt(d.Ts, 10),
			strconv.FormatInt(d.DeviceTs, 10),
			d.Version,
			d.Data,
			d.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	w.Flush()
}

// parseTimeQuery 解析查询参数中的时间（Unix 毫秒、Unix 秒、或 RFC3339）。
func parseTimeQuery(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1_000_000_000_000 { // 毫秒
			t := time.UnixMilli(n)
			return &t, nil
		}
		t := time.Unix(n, 0) // 秒
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ===== 控制日志导出 =====

func (h *Handler) exportControlLogsCSV(c *gin.Context, logs []model.ControlLog) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=control_logs.csv")
	w := csv.NewWriter(c.Writer)
	// UTF-8 BOM，确保 Excel 正确识别中文
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	_ = w.Write([]string{"ID", "设备ID", "消息ID", "控制标签", "状态", "响应码", "响应消息", "创建时间"})
	for _, l := range logs {
		ack := ""
		if l.AckCode != nil {
			ack = strconv.Itoa(*l.AckCode)
		}
		_ = w.Write([]string{
			strconv.FormatUint(uint64(l.ID), 10),
			l.DeviceID,
			l.MsgID,
			l.Tags,
			l.Status,
			ack,
			l.AckMsg,
			l.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	w.Flush()
}

// ===== 控制指令状态查询（控制页轮询/实时回显）=====

type controlStatusView struct {
	MsgID   string `json:"msg_id"`
	Status  string `json:"status"`
	AckCode *int   `json:"ack_code"`
	AckMsg  string `json:"ack_msg"`
}

func (h *Handler) GetControlStatus(c *gin.Context) {
	deviceID := c.Param("deviceId")
	msgID := c.Param("msgId")
	if msgID == "" {
		msgID = c.Query("msg_id")
	}
	clog, err := h.store.GetControlLogByMsgID(msgID)
	if err != nil {
		fail(c, http.StatusNotFound, "指令不存在")
		return
	}
	_, tenantID := getAuthInfo(c)
	if clog.DeviceID != deviceID || !h.assertDeviceBelongsToTenant(clog.DeviceID, tenantID) {
		forbidden(c, "无权查看此指令")
		return
	}
	success(c, controlStatusView{MsgID: clog.MsgID, Status: clog.Status, AckCode: clog.AckCode, AckMsg: clog.AckMsg})
}

// ===== 自定义控制命令（用户可命名、设值、标记高危）=====

func (h *Handler) ListControlCommands(c *gin.Context) {
	projectID, err := parseUintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid project id")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(uint(projectID), tenantID) {
		forbidden(c, "无权操作此项目")
		return
	}
	deviceID := c.Param("deviceId")
	if deviceID == "" {
		deviceID = c.Query("device_id")
	}
	cmds, err := h.store.ListControlCommands(uint(projectID), deviceID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, cmds)
}

func (h *Handler) CreateControlCommand(c *gin.Context) {
	var cmd model.ControlCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(cmd.ProjectID, tenantID) {
		forbidden(c, "无权操作此项目")
		return
	}
	if cmd.Name == "" || cmd.TagKey == "" {
		fail(c, http.StatusBadRequest, "name 和 tag_key 必填")
		return
	}
	if err := h.store.CreateControlCommand(&cmd); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, cmd)
}

func (h *Handler) UpdateControlCommand(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	cmd, err := h.store.GetControlCommand(uint(id))
	if err != nil {
		fail(c, http.StatusNotFound, "指令不存在")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(cmd.ProjectID, tenantID) {
		forbidden(c, "无权操作此项目")
		return
	}
	var body model.ControlCommand
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	cmd.Name = body.Name
	cmd.TagKey = body.TagKey
	cmd.Value = body.Value
	cmd.Icon = body.Icon
	cmd.Sort = body.Sort
	cmd.Danger = body.Danger
	cmd.DeviceID = body.DeviceID
	if err := h.store.UpdateControlCommand(cmd); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, cmd)
}

func (h *Handler) DeleteControlCommand(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	cmd, err := h.store.GetControlCommand(uint(id))
	if err != nil {
		fail(c, http.StatusNotFound, "指令不存在")
		return
	}
	_, tenantID := getAuthInfo(c)
	if !h.assertProjectBelongsToTenant(cmd.ProjectID, tenantID) {
		forbidden(c, "无权操作此项目")
		return
	}
	if err := h.store.DeleteControlCommand(uint(id)); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// ===== 设备启用/禁用（封禁）=====

func (h *Handler) SetDeviceEnabled(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		forbidden(c, "无权操作此设备")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SetDeviceEnabled(deviceID, req.Enabled); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !req.Enabled {
		// 禁用：立即向在线连接下发原因（wendao/{id}/kicked，reason=disabled）并踢除，
		// 不必等设备下次重连；启用不触碰现有连接。broker 不可达时仅通知不踢（静默降级）。
		notifyAndKick(deviceID, "", "disabled", "设备已被管理员禁用，本连接被平台断开", true)
		// 兜底：无论 broker 踢线是否成功，立即广播一次离线状态，前端无需等待断开事件/手动刷新。
		if h.bus != nil {
			if dev, err := h.store.GetDevice(deviceID); err == nil && dev.TenantID != 0 {
				h.bus.BroadcastToTenant(dev.TenantID, events.Message{
					Type: "device_status",
					Data: map[string]interface{}{"device_id": deviceID, "online": false, "enabled": false},
				})
			}
		}
	}
	success(c, gin.H{"id": deviceID, "enabled": req.Enabled})
}

// ===== 设备密钥查看/重置（一机一密）=====

func (h *Handler) ResetDeviceSecret(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		forbidden(c, "无权操作此设备")
		return
	}
	secret, err := cryptopkg.RandomPassword(20)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SetDeviceSecret(deviceID, cryptopkg.MustHashPassword(secret)); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"device_id": deviceID, "device_secret": secret, "secret_note": "明文仅本次返回，请妥善保存"})
}

// ===== 设备数据删除（仅超管，路由级 RequireRole） =====

// DeleteDeviceData DELETE /devices/:deviceId/data
// body 三选一：
//
//	{"ids": [1,2,3]}                      按行 ID 删除
//	{"start": 1725000000000, "end": ...}  按时间范围（毫秒，可只给一端）
//	{"all": true}                          清空该设备全部数据
func (h *Handler) DeleteDeviceData(c *gin.Context) {
	deviceID := c.Param("deviceId")
	var req struct {
		IDs   []uint `json:"ids"`
		Start int64  `json:"start"`
		End   int64  `json:"end"`
		All   bool   `json:"all"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	var n int64
	var err error
	switch {
	case req.All:
		n, err = h.store.DeleteDeviceDataRange(deviceID, nil, nil)
	case len(req.IDs) > 0:
		n, err = h.store.DeleteDeviceDataByIDs(deviceID, req.IDs)
	case req.Start > 0 || req.End > 0:
		var start, end *time.Time
		if req.Start > 0 {
			t := time.UnixMilli(req.Start)
			start = &t
		}
		if req.End > 0 {
			t := time.UnixMilli(req.End)
			end = &t
		}
		n, err = h.store.DeleteDeviceDataRange(deviceID, start, end)
	default:
		fail(c, http.StatusBadRequest, "请指定删除条件：ids / start+end / all")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"deleted": n})
}
