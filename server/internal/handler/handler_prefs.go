package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// uiPreferences 每账号界面偏好。新增开关时在此结构登记并在 mergeUIPreferences 加合并即可。
// 服务器是偏好的唯一权威存储：GET 返回合并默认值后的完整结构；PUT 与已存值做白名单合并。
type uiPreferences struct {
	// DeviceProjectView 设备管理是否按项目卡片二级浏览；false=进入即显示全部设备（默认）。
	DeviceProjectView bool `json:"device_project_view"`
}

func defaultUIPreferences() uiPreferences {
	return uiPreferences{DeviceProjectView: false}
}

// GetMyPreferences GET /me/preferences：返回当前账号界面偏好（已合并默认值）。
func (h *Handler) GetMyPreferences(c *gin.Context) {
	uid := c.GetUint("user_id")
	raw, err := h.store.GetAdminUserUIOptions(uid)
	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	prefs := defaultUIPreferences()
	if raw != "" {
		// 容忍历史/坏 JSON：解析失败则回落到全部默认，而不是让页面打不开
		_ = json.Unmarshal([]byte(raw), &prefs)
	}
	success(c, prefs)
}

// UpdateMyPreferences PUT /me/preferences：白名单合并保存当前账号界面偏好。
func (h *Handler) UpdateMyPreferences(c *gin.Context) {
	uid := c.GetUint("user_id")

	var req uiPreferences
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	// 与已存值合并，避免未来新增偏好时旧客户端的全量提交把未知键清空。
	merged := defaultUIPreferences()
	if raw, err := h.store.GetAdminUserUIOptions(uid); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &merged)
	}
	merged.DeviceProjectView = req.DeviceProjectView

	out, err := json.Marshal(merged)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.UpdateAdminUserUIOptions(uid, string(out)); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, merged)
}
