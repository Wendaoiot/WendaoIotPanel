package store

import (
	"wendaoiotpanel/internal/model"

	"gorm.io/gorm"
)

// ========================= 设备间通信（D2D） =========================

// CreateDeviceMessage 落库一条设备间消息（含投递结果）。
func (s *Store) CreateDeviceMessage(m *model.DeviceMessage) error {
	return s.db.Create(m).Error
}

// ListDeviceMessages 查询某设备的消息留痕（作为发送方或接收方，双向），
// 按时间倒序，租户作用域由调用方保证。
func (s *Store) ListDeviceMessages(deviceID string, limit, offset int) ([]model.DeviceMessage, int64, error) {
	var (
		rows  []model.DeviceMessage
		total int64
	)
	q := s.db.Model(&model.DeviceMessage{}).
		Where("from_device_id = ? OR to_device_id = ?", deviceID, deviceID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

// GetPeerAllow 查询跨租户白名单授权是否存在。
func (s *Store) GetPeerAllow(fromDeviceID, toDeviceID string) (*model.DevicePeerAllow, error) {
	var a model.DevicePeerAllow
	err := s.db.First(&a, "from_device_id = ? AND to_device_id = ?", fromDeviceID, toDeviceID).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListPeerAllows 白名单全量（管理端）。
func (s *Store) ListPeerAllows(tenantID *uint, limit, offset int) ([]model.DevicePeerAllow, int64, error) {
	var (
		rows  []model.DevicePeerAllow
		total int64
	)
	q := s.db.Model(&model.DevicePeerAllow{})
	if tenantID != nil {
		q = q.Where("tenant_id = ?", *tenantID)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

// CreatePeerAllow 新增白名单；重复授权幂等返回已存在记录。
func (s *Store) CreatePeerAllow(a *model.DevicePeerAllow) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var exist model.DevicePeerAllow
		err := tx.First(&exist, "from_device_id = ? AND to_device_id = ?",
			a.FromDeviceID, a.ToDeviceID).Error
		if err == nil {
			*a = exist
			return nil
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		return tx.Create(a).Error
	})
}

// DeletePeerAllow 删除白名单授权。
func (s *Store) DeletePeerAllow(id uint) error {
	return s.db.Delete(&model.DevicePeerAllow{}, id).Error
}

// ListPeerTargets peer 广播/多播的目标解析：返回发送方同租户内的全部"存在"目标
// （不限在线/启用状态，由调用方按需分类投递），大小写精确匹配（utf8mb4_bin）。
//
//   - projectID 非 nil：限定项目（to = project:{id}）
//   - ids 非空：显式 ID 列表（to = "a,b,c" 多播）
//   - exclude：排除的设备 ID（发送方自己，广播不含自己）
//
// 租户作用域一律通过 project → tenant 关联判定，不依赖 device.tenant_id 冗余字段的回填完整性。
func (s *Store) ListPeerTargets(tenantID uint, projectID *uint, ids []string, exclude string, limit int) ([]model.Device, error) {
	q := s.db.Model(&model.Device{}).
		Where("id <> ?", exclude).
		Where("project_id IN (?)", s.db.Model(&model.Project{}).Select("id").Where("tenant_id = ?", tenantID))
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var rows []model.Device
	err := q.Limit(limit).Find(&rows).Error
	return rows, err
}
