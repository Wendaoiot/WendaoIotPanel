package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"wendaoiotpanel/internal/model"
	"wendaoiotpanel/internal/protocol"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Firmware CRUD

func (h *Handler) CreateFirmware(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Version     string `json:"version" binding:"required"`
		URL         string `json:"url" binding:"required"`
		Size        int64  `json:"size"`
		MD5         string `json:"md5"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}
	if !isHTTPSURL(req.URL) {
		fail(c, http.StatusBadRequest, "固件下载地址必须为 https://")
		return
	}
	fw := &model.Firmware{
		Name:        req.Name,
		Version:     req.Version,
		URL:         req.URL,
		Size:        req.Size,
		MD5:         req.MD5,
		Description: req.Description,
	}
	if err := h.store.CreateFirmware(fw); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, fw)
}

func (h *Handler) ListFirmwares(c *gin.Context) {
	list, err := h.store.ListFirmwares()
	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, list)
}

func (h *Handler) DeleteFirmware(c *gin.Context) {
	var req struct {
		ID uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}
	if err := h.store.DeleteFirmware(req.ID); err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, nil)
}

// OTA 触发

func (h *Handler) CreateOTATask(c *gin.Context) {
	_, tenantID := getAuthInfo(c)
	var req struct {
		FirmwareID uint   `json:"firmware_id" binding:"required"`
		TargetType string `json:"target_type" binding:"required"`
		TargetID   string `json:"target_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, -1, err.Error())
		return
	}

	fw, err := h.store.GetFirmwareByID(req.FirmwareID)
	if err != nil {
		fail(c, -1, "固件不存在")
		return
	}

	var taskID uint
	if req.TargetType == "device" {
		if !h.assertDeviceBelongsToTenant(req.TargetID, tenantID) {
			fail(c, 403, "无权操作此设备")
			return
		}
		taskID, err = h.createDeviceOTA(req.TargetID, fw)
	} else if req.TargetType == "project" {
		pid, _ := strconv.ParseUint(req.TargetID, 10, 64)
		if !h.assertProjectBelongsToTenant(uint(pid), tenantID) {
			fail(c, 403, "无权操作此项目")
			return
		}
		taskID, err = h.createProjectOTA(uint(pid), fw)
	} else {
		fail(c, -1, "target_type必须是device或project")
		return
	}

	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, gin.H{"task_id": taskID})
}

func (h *Handler) createDeviceOTA(deviceID string, fw *model.Firmware) (uint, error) {
	task := &model.OTATask{
		FirmwareID: fw.ID,
		TargetType: "device",
		TargetID:   deviceID,
		Status:     model.OTATaskStatusRunning,
	}
	if err := h.store.CreateOTATask(task); err != nil {
		return 0, err
	}

	msgID := uuid.New().String()
	otaLog := &model.OTALog{
		TaskID:   task.ID,
		DeviceID: deviceID,
		Status:   model.OTALogStatusPending,
	}
	if err := h.store.CreateOTALog(otaLog); err != nil {
	}

	cmd := &protocol.OTARequest{
		ID:      msgID,
		Type:    "ota",
		Version: fw.Version,
		URL:     fw.URL,
		MD5:     fw.MD5,
		Size:    fw.Size,
	}
	payload, _ := json.Marshal(cmd)
	if err := h.mqtt.PublishRaw(protocol.TopicOTA(deviceID), payload); err != nil {
		log.Printf("ota: publish to %s error: %v", deviceID, err)
	}
	return task.ID, nil
}

func (h *Handler) createProjectOTA(projectID uint, fw *model.Firmware) (uint, error) {
	devices, err := h.store.ListDevicesByProject(projectID)
	if err != nil {
		return 0, err
	}

	task := &model.OTATask{
		FirmwareID: fw.ID,
		TargetType: "project",
		TargetID:   fmt.Sprint(projectID),
		Status:     model.OTATaskStatusRunning,
	}
	if err := h.store.CreateOTATask(task); err != nil {
		return 0, err
	}

	for _, d := range devices {
		if !d.Enabled {
			continue
		}
		otaLog := &model.OTALog{
			TaskID:   task.ID,
			DeviceID: d.ID,
			Status:   model.OTALogStatusPending,
		}
		if err := h.store.CreateOTALog(otaLog); err != nil {
		}

		msgID := uuid.New().String()
		cmd := &protocol.OTARequest{
			ID:      msgID,
			Type:    "ota",
			Version: fw.Version,
			URL:     fw.URL,
			MD5:     fw.MD5,
			Size:    fw.Size,
		}
		payload, _ := json.Marshal(cmd)
		if err := h.mqtt.PublishRaw(protocol.TopicOTA(d.ID), payload); err != nil {
			log.Printf("ota: publish to %s error: %v", d.ID, err)
		}
	}

	return task.ID, nil
}

func (h *Handler) ListOTATasks(c *gin.Context) {
	_, tenantID := getAuthInfo(c)
	tasks, err := h.store.ListOTATasksScoped(tenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, tasks)
}

func (h *Handler) GetOTALogs(c *gin.Context) {
	var req struct {
		TaskID uint `form:"task_id"`
	}
	c.ShouldBindQuery(&req)
	_, tenantID := getAuthInfo(c)
	if tenantID != nil && req.TaskID > 0 {
		task, err := h.store.GetOTATaskByID(req.TaskID)
		if err != nil {
			fail(c, http.StatusNotFound, "任务不存在")
			return
		}
		if !h.assertOTATaskBelongsToTenant(task, *tenantID) {
			forbidden(c, "无权查看此任务")
			return
		}
	}
	logs, err := h.store.ListOTALogs(req.TaskID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, logs)
}

func (h *Handler) GetDeviceFirmware(c *gin.Context) {
	var req struct {
		Version string `form:"version"`
	}
	c.ShouldBindQuery(&req)

	var fw *model.Firmware
	var err error
	if req.Version != "" {
		fw, err = h.store.GetFirmwareByVersion(req.Version)
	} else {
		fw, err = h.store.GetLatestFirmware()
	}
	if err != nil {
		fail(c, -1, "固件不存在")
		return
	}

	success(c, fw)
}

func (h *Handler) assertDeviceBelongsToTenant(deviceID string, tenantID *uint) bool {
	if tenantID == nil {
		return true
	}
	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		return false
	}
	return h.assertProjectBelongsToTenant(dev.ProjectID, tenantID)
}

// assertOTATaskBelongsToTenant 校验 OTA 任务目标（设备/项目）归属当前租户。
func (h *Handler) assertOTATaskBelongsToTenant(task *model.OTATask, tenantID uint) bool {
	if task.TargetType == "device" {
		dev, err := h.store.GetDevice(task.TargetID)
		if err != nil {
			return false
		}
		return h.assertProjectBelongsToTenant(dev.ProjectID, &tenantID)
	}
	// project
	pid, err := strconv.ParseUint(task.TargetID, 10, 64)
	if err != nil {
		return false
	}
	return h.assertProjectBelongsToTenant(uint(pid), &tenantID)
}
