package handler

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== 日志删除（路由级强制仅超管；物理删除） =====

// DeleteControlLogs DELETE /control-logs
// body 三选一：
//
//	{"ids":[1,2,3]}                                按行 ID 删除（表格多选）
//	{"device_id":"x","start":ms,"end":ms}         按设备/时间范围清空（字段均可只给一部分）
//	{"all":true,"device_id":"x","start":ms,...}   按筛选清空；无任何筛选条件时=清空全部
//
// 非 all 模式下使用过滤删除时，至少要给出 device_id / start / end 之一，防止误清空。
func (h *Handler) DeleteControlLogs(c *gin.Context) {
	var req struct {
		IDs      []uint `json:"ids"`
		DeviceID string `json:"device_id"`
		Start    int64  `json:"start"`
		End      int64  `json:"end"`
		All      bool   `json:"all"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	var n int64
	var err error
	switch {
	case len(req.IDs) > 0:
		n, err = h.store.DeleteControlLogsByIDs(req.IDs)
	// all=true 且无任何筛选条件=清空全部；带筛选=清空当前筛选结果。
	// 非 all 模式至少给出一个筛选条件，防止误清空。
	case req.All || req.DeviceID != "" || req.Start > 0 || req.End > 0:
		var start, end *time.Time
		if req.Start > 0 {
			t := time.UnixMilli(req.Start)
			start = &t
		}
		if req.End > 0 {
			t := time.UnixMilli(req.End)
			end = &t
		}
		n, err = h.store.DeleteControlLogsByFilter(req.DeviceID, start, end)
	default:
		fail(c, http.StatusBadRequest, "请指定删除条件：ids / device_id+时间范围 / all")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"deleted": n})
}

// DeleteOTALogs DELETE /ota/logs
// body 三选一：
//
//	{"ids":[1,2,3]}   按行 ID 删除
//	{"task_id":7}      清空某升级任务下的全部设备日志
//	{"all":true}       清空全部 OTA 升级日志
//
// 仅删除升级日志，不删除 OTA 任务本身；运行中任务的日志删除不影响其状态机。
func (h *Handler) DeleteOTALogs(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids"`
		TaskID uint   `json:"task_id"`
		All    bool   `json:"all"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	var n int64
	var err error
	switch {
	case len(req.IDs) > 0:
		n, err = h.store.DeleteOTALogsByIDs(req.IDs)
	case req.TaskID > 0:
		n, err = h.store.DeleteOTALogsByTaskID(req.TaskID)
	case req.All:
		n, err = h.store.DeleteOTALogsByTaskID(0)
	default:
		fail(c, http.StatusBadRequest, "请指定删除条件：ids / task_id / all")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{"deleted": n})
}
