package store

import (
	"database/sql"
	"time"
	"wendaoiotpanel/internal/model"

	"gorm.io/gorm"
)

// ========================= AdminUser / 认证 =========================

// GetAdminUserByIDFull 返回含 TokenVersion 的用户（仍不返回密码），供中间件校验。
func (s *Store) GetAdminUserByIDFull(id uint) (*model.AdminUser, error) {
	var u model.AdminUser
	if err := s.db.Omit("password").First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateAdminUserPasswordAndBump 更新密码哈希并递增 token 版本（使旧 token 失效）。
func (s *Store) UpdateAdminUserPasswordAndBump(id uint, hashedPassword string) error {
	return s.db.Model(&model.AdminUser{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"password":        hashedPassword,
			"token_version":   gorm.Expr("token_version + 1"),
			"pass_changed_at": time.Now(),
		}).Error
}

// GetAdminUserUIOptions 返回某账号原始 UI 偏好 JSON 串（空串=全部默认）。
// ui_options 允许 NULL（从未保存过偏好的账号，如 admin）：NULL 按空串处理，不报错。
func (s *Store) GetAdminUserUIOptions(id uint) (string, error) {
	var opts sql.NullString
	err := s.db.Raw("SELECT ui_options FROM admin_users WHERE id = ?", id).Scan(&opts).Error
	if err != nil {
		return "", err
	}
	return opts.String, nil
}

// UpdateAdminUserUIOptions 保存某账号原始 UI 偏好 JSON 串。
func (s *Store) UpdateAdminUserUIOptions(id uint, optsJSON string) error {
	return s.db.Model(&model.AdminUser{}).Where("id = ?", id).Update("ui_options", optsJSON).Error
}

// BumpTokenVersion 递增用户 token 版本（如下线/强制重新登录）。
func (s *Store) BumpTokenVersion(id uint) error {
	return s.db.Model(&model.AdminUser{}).Where("id = ?", id).
		Update("token_version", gorm.Expr("token_version + 1")).Error
}

// ========================= Device / 接入认证 =========================

// GetDeviceFull 返回设备完整信息（含接入密钥，用于 broker 认证；不在 JSON 暴露）。
func (s *Store) GetDeviceFull(deviceID string) (*model.Device, error) {
	var d model.Device
	if err := s.db.First(&d, "id = ?", deviceID).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// GetDeviceByRecord 供 broker 认证：按设备 ID 查找，仅取接入相关字段。
func (s *Store) GetDeviceAuthRecord(deviceID string) (*model.Device, error) {
	return s.GetDeviceFull(deviceID)
}

func (s *Store) CreateDeviceWithSecret(d *model.Device, secretHash string) error {
	d.DeviceSecret = secretHash
	d.Enabled = true
	return s.db.Create(d).Error
}

func (s *Store) SetDeviceSecret(deviceID string, secretHash string) error {
	return s.db.Model(&model.Device{}).Where("id = ?", deviceID).
		Update("device_secret", secretHash).Error
}

func (s *Store) SetDeviceEnabled(deviceID string, enabled bool) error {
	status := model.DeviceStatusOffline
	if !enabled {
		// 禁用即视为拉黑，状态置为 Inactive（不在线、不可控）
		status = model.DeviceStatusInactive
	}
	return s.db.Model(&model.Device{}).Where("id = ?", deviceID).
		Updates(map[string]interface{}{"enabled": enabled, "status": status}).Error
}

// TenantIDForDevice 通过 project → device 冗余字段取租户。
func (s *Store) SetDeviceRuntimeStatus(deviceID string, online bool) error {
	status := model.DeviceStatusOffline
	if online {
		status = model.DeviceStatusOnline
	}
	return s.db.Model(&model.Device{}).Where("id = ? AND enabled = ?", deviceID, true).
		Updates(map[string]interface{}{"status": status, "last_active": time.Now()}).Error
}

// ========================= ControlCommand 自定义控制指令 =========================

func (s *Store) CreateControlCommand(c *model.ControlCommand) error {
	return s.db.Create(c).Error
}

func (s *Store) ListControlCommands(projectID uint, deviceID string) ([]model.ControlCommand, error) {
	var cmds []model.ControlCommand
	q := s.db.Where("project_id = ?", projectID)
	q = q.Where("device_id = ? OR device_id = ?", deviceID, "")
	if err := q.Order("sort ASC, id ASC").Find(&cmds).Error; err != nil {
		return nil, err
	}
	return cmds, nil
}

func (s *Store) GetControlCommand(id uint) (*model.ControlCommand, error) {
	var c model.ControlCommand
	if err := s.db.First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpdateControlCommand(c *model.ControlCommand) error {
	return s.db.Save(c).Error
}

func (s *Store) DeleteControlCommand(id uint) error {
	return s.db.Delete(&model.ControlCommand{}, id).Error
}

// ========================= ControlLog 状态机与租户作用域 =========================

func (s *Store) CreateControlLogScoped(l *model.ControlLog) error {
	if l.Status == "" {
		l.Status = model.ControlStatusPending
	}
	return s.db.Create(l).Error
}

func (s *Store) SetControlLogDeliveredOrErr(msgID string, delivered bool) error {
	status := model.ControlStatusTimeout
	if delivered {
		status = model.ControlStatusDelivered
	}
	return s.db.Model(&model.ControlLog{}).Where("msg_id = ? AND status = ?", msgID, model.ControlStatusPending).
		Update("status", status).Error
}

// ApplyControlAck 设备回 ack：code=0 成功，非 0 失败。
func (s *Store) ApplyControlAck(msgID string, code int, msg string) error {
	status := model.ControlStatusSuccess
	if code != 0 {
		status = model.ControlStatusFailed
	}
	res := s.db.Model(&model.ControlLog{}).Where("msg_id = ?", msgID).
		Updates(map[string]interface{}{
			"ack_code": code,
			"ack_msg":  msg,
			"status":   status,
		})
	if res.Error != nil {
		return res.Error
	}
	// 没有任何指令匹配该 msg_id：回执 id 为平台未知，上层应答 code=3。
	// GORM Updates 匹配 0 行时 Error 仍为 nil，必须显式检查 RowsAffected。
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MarkControlTimeout 将超过阈值仍未得到 ack 的指令标记为超时。
func (s *Store) MarkControlTimeout(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	res := s.db.Model(&model.ControlLog{}).
		Where("status IN ? AND created_at < ?", []string{model.ControlStatusPending, model.ControlStatusDelivered}, cutoff).
		Update("status", model.ControlStatusTimeout)
	return res.RowsAffected, res.Error
}

// ListControlLogsScoped 按租户作用域查询控制日志，支持设备过滤与时间范围。
func (s *Store) ListControlLogsScoped(tenantID *uint, deviceID string, start, end *time.Time, limit, offset int) ([]model.ControlLog, int64, error) {
	q := s.db.Model(&model.ControlLog{})
	if tenantID != nil {
		q = q.Where("tenant_id = ?", *tenantID)
	}
	if deviceID != "" {
		q = q.Where("device_id = ?", deviceID)
	}
	if start != nil {
		q = q.Where("created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("created_at <= ?", *end)
	}
	var total int64
	q.Count(&total)
	var logs []model.ControlLog
	err := q.Order("id desc").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, total, err
}

// ========================= DeviceData 导出 / 时间范围 =========================

func (s *Store) ListDeviceDataRange(deviceID string, start, end *time.Time, limit, offset int) ([]model.DeviceData, int64, error) {
	q := s.db.Model(&model.DeviceData{}).Where("device_id = ?", deviceID)
	if start != nil {
		q = q.Where("ts >= ?", start.UnixMilli())
	}
	if end != nil {
		q = q.Where("ts <= ?", end.UnixMilli())
	}
	var total int64
	q.Count(&total)
	var data []model.DeviceData
	err := q.Order("ts desc").Limit(limit).Offset(offset).Find(&data).Error
	return data, total, err
}

// ========================= OTA 租户作用域 =========================

func (s *Store) ListOTATasksScoped(tenantID *uint) ([]model.OTATask, error) {
	var tasks []model.OTATask
	q := s.db.Model(&model.OTATask{})
	if tenantID != nil {
		// OTA 任务目标为 project 时按租户项目集合过滤；device 时按设备冗余字段。
		var projectIDs []uint
		s.db.Model(&model.Project{}).Where("tenant_id = ?", *tenantID).Pluck("id", &projectIDs)
		var deviceIDs []string
		s.db.Model(&model.Device{}).Where("tenant_id = ?", *tenantID).Pluck("id", &deviceIDs)
		q = q.Where(
			"(target_type = 'project' AND target_id IN ?) OR (target_type = 'device' AND target_id IN ?)",
			uintsToStrings(projectIDs), deviceIDs,
		)
	}
	err := q.Order("id desc").Find(&tasks).Error
	return tasks, err
}

// GetOTATaskByID 按 ID 查询 OTA 任务（含任务目标信息，用于归属校验）。
func (s *Store) GetOTATaskByID(id uint) (*model.OTATask, error) {
	var t model.OTATask
	if err := s.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ========================= Tenant 作用域项目/设备工具 =========================

func (s *Store) TenantIDForDeviceID(deviceID string) (uint, error) {
	var d model.Device
	if err := s.db.Select("tenant_id").First(&d, "id = ?", deviceID).Error; err != nil {
		return 0, err
	}
	return d.TenantID, nil
}

func (s *Store) SetDeviceTenant(deviceID string, tenantID uint) error {
	return s.db.Model(&model.Device{}).Where("id = ?", deviceID).Update("tenant_id", tenantID).Error
}

// GetDeviceTagByID 按主键查询设备标签（用于归属校验后删除）。
func (s *Store) GetDeviceTagByID(id uint) (*model.DeviceTag, error) {
	var t model.DeviceTag
	if err := s.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// GetProjectTagByID 按主键查询项目标签（用于归属校验后删除）。
func (s *Store) GetProjectTagByID(id uint) (*model.ProjectTag, error) {
	var t model.ProjectTag
	if err := s.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateDeviceTag 更新设备标签（含备注名/单位）。
func (s *Store) UpdateDeviceTag(t *model.DeviceTag) error {
	return s.db.Save(t).Error
}

// ========================= DeviceData 删除（仅超管） =========================

// DeleteDeviceDataByIDs 物理删除指定 ID 的数据行（id 必须属于该设备）。
func (s *Store) DeleteDeviceDataByIDs(deviceID string, ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := s.db.Unscoped().Where("device_id = ? AND id IN ?", deviceID, ids).Delete(&model.DeviceData{})
	return res.RowsAffected, res.Error
}

// DeleteDeviceDataRange 物理删除某设备 ts 在 [start, end] 内的数据行（nil 边界=不限制）。
func (s *Store) DeleteDeviceDataRange(deviceID string, start, end *time.Time) (int64, error) {
	q := s.db.Unscoped().Where("device_id = ?", deviceID)
	if start != nil {
		q = q.Where("ts >= ?", start.UnixMilli())
	}
	if end != nil {
		q = q.Where("ts <= ?", end.UnixMilli())
	}
	res := q.Delete(&model.DeviceData{})
	return res.RowsAffected, res.Error
}

// CountDeviceData 返回某设备当前数据行数（删除确认弹窗展示用）。
func (s *Store) CountDeviceData(deviceID string) (int64, error) {
	var n int64
	err := s.db.Model(&model.DeviceData{}).Where("device_id = ?", deviceID).Count(&n).Error
	return n, err
}

// ========================= 日志删除（仅超管，物理删除） =========================

// DeleteControlLogsByIDs 按主键批量删除控制日志（行 ID 全局唯一，仅限超管调用）。
func (s *Store) DeleteControlLogsByIDs(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := s.db.Where("id IN ?", ids).Delete(&model.ControlLog{})
	return res.RowsAffected, res.Error
}

// DeleteControlLogsByFilter 按设备/时间范围删除控制日志；deviceID 为空、start/end 为 nil 表示不限制
// （三者全空即清空全部）。
func (s *Store) DeleteControlLogsByFilter(deviceID string, start, end *time.Time) (int64, error) {
	q := s.db.Model(&model.ControlLog{})
	if deviceID != "" {
		q = q.Where("device_id = ?", deviceID)
	}
	if start != nil {
		q = q.Where("created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("created_at <= ?", *end)
	}
	res := q.Delete(&model.ControlLog{})
	return res.RowsAffected, res.Error
}

// DeleteOTALogsByIDs 按主键批量删除 OTA 升级日志（仅限超管调用）。
func (s *Store) DeleteOTALogsByIDs(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := s.db.Where("id IN ?", ids).Delete(&model.OTALog{})
	return res.RowsAffected, res.Error
}

// DeleteOTALogsByTaskID 清空某升级任务下的全部设备日志；taskID 为 0 表示清空全部 OTA 日志。
func (s *Store) DeleteOTALogsByTaskID(taskID uint) (int64, error) {
	q := s.db.Model(&model.OTALog{})
	if taskID > 0 {
		q = q.Where("task_id = ?", taskID)
	}
	res := q.Delete(&model.OTALog{})
	return res.RowsAffected, res.Error
}
