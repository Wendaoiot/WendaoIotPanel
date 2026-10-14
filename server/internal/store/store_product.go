package store

import (
	"errors"
	"time"

	"wendaoiotpanel/internal/model"

	"gorm.io/gorm"
)

// ErrProductInUse 产品下仍有设备引用时拒绝删除（设备需先迁移或删除）。
var ErrProductInUse = errors.New("产品下仍有设备，不能删除")

// ========================= Product（一型一密） =========================

// CreateProduct 创建产品（ProductSecret 由 handler 生成 bcrypt 哈希后写入）。
func (s *Store) CreateProduct(p *model.Product) error {
	return s.db.Create(p).Error
}

// GetProductByKey 按 ProductKey 查询（含密钥哈希，供引导认证使用）。
func (s *Store) GetProductByKey(productKey string) (*model.Product, error) {
	var p model.Product
	if err := s.db.First(&p, "product_key = ?", productKey).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProductByID 按主键查询产品。
func (s *Store) GetProductByID(id uint) (*model.Product, error) {
	var p model.Product
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProducts 列产品：tenantID=nil 查全部（超管），否则限本租户。
func (s *Store) ListProducts(tenantID *uint) ([]model.Product, error) {
	var products []model.Product
	q := s.db.Order("id")
	if tenantID != nil {
		q = q.Where("tenant_id = ?", *tenantID)
	}
	err := q.Find(&products).Error
	return products, err
}

// UpdateProduct 更新产品名称/动态注册开关（nil 表示该字段不改）。
func (s *Store) UpdateProduct(productKey, name string, dynRegEnabled *bool) error {
	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	if dynRegEnabled != nil {
		updates["dyn_reg_enabled"] = *dynRegEnabled
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(&model.Product{}).Where("product_key = ?", productKey).Updates(updates).Error
}

// SetProductSecret 按 ProductKey 重置产品密钥（bcrypt 哈希）。
func (s *Store) SetProductSecret(productKey, secretHash string) error {
	return s.db.Model(&model.Product{}).Where("product_key = ?", productKey).
		Update("product_secret", secretHash).Error
}

// CountDevicesByProduct 统计产品下设备数（含待激活与已激活），用于删除保护。
func (s *Store) CountDevicesByProduct(productID uint) (int64, error) {
	var n int64
	err := s.db.Model(&model.Device{}).Where("product_id = ?", productID).Count(&n).Error
	return n, err
}

// DeleteProductByKey 删除产品：有设备引用则拒绝；通过后物理删除以释放 ProductKey
// （product_key 为不带 deleted_at 的唯一索引，软删行会阻塞同名重建）。
func (s *Store) DeleteProductByKey(productKey string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p model.Product
		if err := tx.First(&p, "product_key = ?", productKey).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.Device{}).Where("product_id = ?", p.ID).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrProductInUse
		}
		return tx.Unscoped().Where("product_key = ?", productKey).Delete(&model.Product{}).Error
	})
}

// ========================= 设备预录 / 动态激活 =========================

// PreregisterDevice 预录待激活设备：绑定产品，密钥留空。
// DeviceSecret 为空即"待激活"状态，普通一机一密认证拒绝，仅引导连接可激活。
// 设备 ID 冲突（1062）由调用方转为逐行失败结果。
func (s *Store) PreregisterDevice(d *model.Device) error {
	d.DeviceSecret = ""
	d.ActivatedAt = nil
	d.Enabled = true
	return s.db.Create(d).Error
}

// ActivateDevice 条件激活：仅当设备属于该产品、启用且尚未持有密钥时写入一机一密。
// 条件更新保证并发引导注册只有一个赢标（RowsAffected=1）；返回赢标与否及最新设备行。
func (s *Store) ActivateDevice(sn string, productID uint, secretHash string) (bool, *model.Device, error) {
	var won bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Device{}).
			Where("id = ? AND product_id = ? AND enabled = ? AND (device_secret = '' OR device_secret IS NULL)",
				sn, productID, true).
			Updates(map[string]interface{}{
				"device_secret": secretHash,
				"activated_at":  time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		won = res.RowsAffected == 1
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	d, gerr := s.GetDevice(sn)
	if gerr != nil {
		return won, nil, gerr
	}
	return won, d, nil
}

// ClearDeviceActivation 重新允许动态注册（reactivate）：清空一机一密与激活时间，
// 设备回到待激活状态，旧密钥立即失效；仅限产品设备（product_id > 0）。
// 无匹配行返回 gorm.ErrRecordNotFound。
func (s *Store) ClearDeviceActivation(deviceID string) error {
	res := s.db.Model(&model.Device{}).
		Where("id = ? AND product_id > 0", deviceID).
		Updates(map[string]interface{}{
			"device_secret": "",
			"activated_at":  nil,
			"status":        model.DeviceStatusOffline,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
