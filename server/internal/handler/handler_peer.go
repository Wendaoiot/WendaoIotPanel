package handler

import (
	"net/http"
	"strconv"

	"wendaoiotpanel/internal/model"

	"github.com/gin-gonic/gin"
)

// ========================= 设备间通信（D2D）管理端 API =========================

// ListPeerMessages GET /devices/:deviceId/peer/messages?limit=&offset=
// 查询设备收发的设备间消息留痕（双向），租户作用域。
func (h *Handler) ListPeerMessages(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	if !h.assertDeviceBelongsToTenant(deviceID, tenantID) {
		forbidden(c, "无权查看此设备的消息")
		return
	}
	limit := 50
	offset := 0
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		offset = v
	}
	rows, total, err := h.store.ListDeviceMessages(deviceID, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"list": rows, "total": total, "limit": limit, "offset": offset})
}

// ListPeerAllows GET /peer/allows — 白名单列表（超管看全部，租户管理员看本租户发起的）。
func (h *Handler) ListPeerAllows(c *gin.Context) {
	role, tenantID := getAuthInfo(c)
	limit := 100
	offset := 0
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		offset = v
	}
	var scope *uint
	if role != model.RoleSuperAdmin {
		scope = tenantID
	}
	rows, total, err := h.store.ListPeerAllows(scope, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"list": rows, "total": total, "limit": limit, "offset": offset})
}

// CreatePeerAllow POST /peer/allows — 新增跨租户授权 {from_device_id, to_device_id, remark}。
func (h *Handler) CreatePeerAllow(c *gin.Context) {
	_, tenantID := getAuthInfo(c)
	var req struct {
		FromDeviceID string `json:"from_device_id" binding:"required"`
		ToDeviceID   string `json:"to_device_id" binding:"required"`
		Remark       string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	// 双方设备都必须存在，且发起方必须属于当前租户（超管可跨租户授权任意对）
	fromDev, err := h.store.GetDevice(req.FromDeviceID)
	if err != nil {
		fail(c, http.StatusBadRequest, "发送方设备不存在")
		return
	}
	if _, err := h.store.GetDevice(req.ToDeviceID); err != nil {
		fail(c, http.StatusBadRequest, "目标设备不存在")
		return
	}
	if tenantID != nil && fromDev.TenantID != *tenantID {
		forbidden(c, "只能为本租户设备发起的通信授权")
		return
	}
	a := &model.DevicePeerAllow{
		FromDeviceID: req.FromDeviceID,
		ToDeviceID:   req.ToDeviceID,
		TenantID:     fromDev.TenantID,
		Remark:       req.Remark,
	}
	if err := h.store.CreatePeerAllow(a); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, a)
}

// DeletePeerAllow DELETE /peer/allows/:id — 删除授权。
func (h *Handler) DeletePeerAllow(c *gin.Context) {
	role, tenantID := getAuthInfo(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	// 非超管只能删本租户发起的授权
	if role != model.RoleSuperAdmin {
		if tenantID == nil {
			forbidden(c, "无权操作")
			return
		}
		rows, _, err := h.store.ListPeerAllows(tenantID, 1000, 0)
		if err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		found := false
		for _, r := range rows {
			if r.ID == uint(id) {
				found = true
				break
			}
		}
		if !found {
			forbidden(c, "无权删除此授权")
			return
		}
	}
	if err := h.store.DeletePeerAllow(uint(id)); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"deleted": id})
}
