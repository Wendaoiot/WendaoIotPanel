package store

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"wendaoiotpanel/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func New(dsn string) (*Store, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) AutoMigrate() error {
	return s.db.AutoMigrate(
		&model.Tenant{},
		&model.Project{},
		&model.Product{},
		&model.Device{},
		&model.DeviceTag{},
		&model.ProjectTag{},
		&model.DeviceData{},
		&model.ControlLog{},
		&model.AdminUser{},
		&model.Firmware{},
		&model.OTATask{},
		&model.OTALog{},
		&model.ControlCommand{},
		&model.DeviceMessage{},
		&model.DevicePeerAllow{},
	)
}

// AdminUser

func (s *Store) GetAdminUserByUsername(username string) (*model.AdminUser, error) {
	var u model.AdminUser
	err := s.db.First(&u, "username = ?", username).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) CreateAdminUser(u *model.AdminUser) error {
	return s.db.Create(u).Error
}

func (s *Store) ListAdminUsers(tenantID *uint) ([]model.AdminUser, error) {
	var users []model.AdminUser
	q := s.db.Omit("password")
	if tenantID != nil {
		q = q.Where("tenant_id = ?", *tenantID)
	}
	err := q.Order("id").Find(&users).Error
	return users, err
}

func (s *Store) GetAdminUserByID(id uint) (*model.AdminUser, error) {
	var u model.AdminUser
	err := s.db.Omit("password").First(&u, id).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) UpdateAdminUserPassword(id uint, hashedPassword string) error {
	return s.db.Model(&model.AdminUser{}).Where("id = ?", id).Update("password", hashedPassword).Error
}

func (s *Store) DeleteAdminUser(id uint) error {
	return s.db.Delete(&model.AdminUser{}, id).Error
}

// Tenant

// CreateTenant 创建租户，ID 取最低空闲位：删除后新建可复用低位 ID（如全删后从 1 重新开始）。
func (s *Store) CreateTenant(t *model.Tenant) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			var ids []uint
			if err := tx.Unscoped().Model(&model.Tenant{}).Pluck("id", &ids).Error; err != nil {
				return err
			}
			used := make(map[uint]bool, len(ids))
			for _, id := range ids {
				used[id] = true
			}
			next := uint(1)
			for used[next] {
				next++
			}
			t.ID = next
			return tx.Create(t).Error
		})
		if err == nil {
			return nil
		}
		// 并发抢占同一最低位：重算后重试
		if !strings.Contains(err.Error(), "1062") && !strings.Contains(err.Error(), "Duplicate") {
			return err
		}
	}
	return errors.New("创建租户失败：ID 分配并发冲突，请重试")
}

// TenantNameExists 检查租户名是否已被活跃租户占用（只看未删行，软删残留不算）。
func (s *Store) TenantNameExists(name string) (bool, error) {
	var n int64
	err := s.db.Model(&model.Tenant{}).Where("name = ?", name).Count(&n).Error
	return n > 0, err
}

func (s *Store) ListTenants() ([]model.Tenant, error) {
	var tenants []model.Tenant
	err := s.db.Find(&tenants).Error
	return tenants, err
}

func (s *Store) GetTenantByID(id uint) (*model.Tenant, error) {
	var t model.Tenant
	err := s.db.First(&t, id).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) UpdateTenant(id uint, name string) error {
	return s.db.Model(&model.Tenant{}).Where("id = ?", id).Update("name", name).Error
}

func (s *Store) DeleteTenant(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 物理删除（Unscoped）：租户及子树全部清除，不留软删残留，
		// 名称与 ID 随之释放（配合 CreateTenant 最低位分配实现复用）。
		var projectIDs []uint
		if r := tx.Unscoped().Model(&model.Project{}).Where("tenant_id = ?", id).Pluck("id", &projectIDs); r.Error != nil {
			return r.Error
		}
		if len(projectIDs) > 0 {
			var deviceIDs []string
			if r := tx.Unscoped().Model(&model.Device{}).Where("project_id IN ?", projectIDs).Pluck("id", &deviceIDs); r.Error != nil {
				return r.Error
			}
			if len(deviceIDs) > 0 {
				if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.DeviceTag{}); r.Error != nil {
					return r.Error
				}
				if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.DeviceData{}); r.Error != nil {
					return r.Error
				}
				if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.ControlLog{}); r.Error != nil {
					return r.Error
				}
				if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.OTALog{}); r.Error != nil {
					return r.Error
				}
				// 设备间消息：任一端（发/收）命中即清
				if r := tx.Unscoped().Where("from_device_id IN ? OR to_device_id IN ?", deviceIDs, deviceIDs).Delete(&model.DeviceMessage{}); r.Error != nil {
					return r.Error
				}
				if r := tx.Unscoped().Where("from_device_id IN ? OR to_device_id IN ?", deviceIDs, deviceIDs).Delete(&model.DevicePeerAllow{}); r.Error != nil {
					return r.Error
				}
				if r := tx.Unscoped().Where("id IN ?", deviceIDs).Delete(&model.Device{}); r.Error != nil {
					return r.Error
				}
			}
			// 项目维度子表：自定义控制按钮、项目标签
			if r := tx.Unscoped().Where("project_id IN ?", projectIDs).Delete(&model.ControlCommand{}); r.Error != nil {
				return r.Error
			}
			if r := tx.Unscoped().Where("project_id IN ?", projectIDs).Delete(&model.ProjectTag{}); r.Error != nil {
				return r.Error
			}
			if r := tx.Unscoped().Where("target_type = ? AND target_id IN ?", "project", uintsToStrings(projectIDs)).Delete(&model.OTATask{}); r.Error != nil {
				return r.Error
			}
			// 删除项目：条件必须是主键 id IN ?（此前误写 project_id IN ? 导致 1054 未知列错误被静默吞掉，项目全部残留）
			if r := tx.Unscoped().Where("id IN ?", projectIDs).Delete(&model.Project{}); r.Error != nil {
				return r.Error
			}
		}
		if r := tx.Unscoped().Where("tenant_id = ?", id).Delete(&model.AdminUser{}); r.Error != nil {
			return r.Error
		}
		// 租户的产品（一型一密）在其设备随项目全部清除后删除；
		// 设备均归属于项目且已在上方级联清除，此处不会再有引用。
		if r := tx.Unscoped().Where("tenant_id = ?", id).Delete(&model.Product{}); r.Error != nil {
			return r.Error
		}
		if r := tx.Unscoped().Delete(&model.Tenant{}, id); r.Error != nil {
			return r.Error
		}
		return nil
	})
}

// Project

// CreateProject 创建项目，ID 取最低空闲位（与 CreateTenant 同策略）：
// 删除项目后新建可复用低位 ID（如全删后从 1 重新开始）。并发抢占同一最低位时重试。
func (s *Store) CreateProject(p *model.Project) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			var ids []uint
			if err := tx.Unscoped().Model(&model.Project{}).Pluck("id", &ids).Error; err != nil {
				return err
			}
			used := make(map[uint]bool, len(ids))
			for _, id := range ids {
				used[id] = true
			}
			next := uint(1)
			for used[next] {
				next++
			}
			p.ID = next
			return tx.Create(p).Error
		})
		if err == nil {
			return nil
		}
		// 并发抢占同一最低位：重算后重试
		if !strings.Contains(err.Error(), "1062") && !strings.Contains(err.Error(), "Duplicate") {
			return err
		}
	}
	return errors.New("创建项目失败：ID 分配并发冲突，请重试")
}

func (s *Store) GetProjectByID(id uint) (*model.Project, error) {
	var p model.Project
	err := s.db.First(&p, id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListProjectsByTenant(tenantID uint) ([]model.Project, error) {
	var projects []model.Project
	err := s.db.Where("tenant_id = ?", tenantID).Find(&projects).Error
	return projects, err
}

func (s *Store) ListAllProjects() ([]model.Project, error) {
	var projects []model.Project
	err := s.db.Find(&projects).Error
	return projects, err
}

func (s *Store) UpdateProject(id uint, name string) error {
	return s.db.Model(&model.Project{}).Where("id = ?", id).Update("name", name).Error
}

// UpdateProjectSettings 更新项目级在线判定默认（mode 传 ” 表示沿用系统默认）。
func (s *Store) UpdateProjectSettings(id uint, onlineMode string, offlineTimeoutSec int) error {
	return s.db.Model(&model.Project{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"online_mode":         onlineMode,
			"offline_timeout_sec": offlineTimeoutSec,
		}).Error
}

// ApplyProjectOnlineDefault 将项目在线判定默认显式写入该项目全部设备
// （一键应用：此后设备不再动态跟随项目默认，而是各自持有相同的显式值）。返回受影响设备数。
func (s *Store) ApplyProjectOnlineDefault(projectID uint, onlineMode string, offlineTimeoutSec int) (int64, error) {
	r := s.db.Model(&model.Device{}).
		Where("project_id = ?", projectID).
		Updates(map[string]interface{}{
			"online_mode":         onlineMode,
			"offline_timeout_sec": offlineTimeoutSec,
		})
	return r.RowsAffected, r.Error
}

func (s *Store) DeleteProject(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var deviceIDs []string
		if r := tx.Unscoped().Model(&model.Device{}).Where("project_id = ?", id).Pluck("id", &deviceIDs); r.Error != nil {
			return r.Error
		}
		if len(deviceIDs) > 0 {
			if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.DeviceTag{}); r.Error != nil {
				return r.Error
			}
			if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.DeviceData{}); r.Error != nil {
				return r.Error
			}
			if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.ControlLog{}); r.Error != nil {
				return r.Error
			}
			if r := tx.Unscoped().Where("device_id IN ?", deviceIDs).Delete(&model.OTALog{}); r.Error != nil {
				return r.Error
			}
			// 设备间消息：任一端（发/收）命中即清
			if r := tx.Unscoped().Where("from_device_id IN ? OR to_device_id IN ?", deviceIDs, deviceIDs).Delete(&model.DeviceMessage{}); r.Error != nil {
				return r.Error
			}
			if r := tx.Unscoped().Where("from_device_id IN ? OR to_device_id IN ?", deviceIDs, deviceIDs).Delete(&model.DevicePeerAllow{}); r.Error != nil {
				return r.Error
			}
			if r := tx.Unscoped().Where("id IN ?", deviceIDs).Delete(&model.Device{}); r.Error != nil {
				return r.Error
			}
		}
		// 项目维度子表：自定义控制按钮、项目标签
		if r := tx.Unscoped().Where("project_id = ?", id).Delete(&model.ControlCommand{}); r.Error != nil {
			return r.Error
		}
		if r := tx.Unscoped().Where("project_id = ?", id).Delete(&model.ProjectTag{}); r.Error != nil {
			return r.Error
		}
		if r := tx.Unscoped().Where("target_type = ? AND target_id = ?", "project", strconv.FormatUint(uint64(id), 10)).Delete(&model.OTATask{}); r.Error != nil {
			return r.Error
		}
		return tx.Unscoped().Delete(&model.Project{}, id).Error
	})
}

// Device

func (s *Store) CreateDevice(d *model.Device) error {
	return s.db.Create(d).Error
}

func (s *Store) GetDevice(deviceID string) (*model.Device, error) {
	var d model.Device
	err := s.db.First(&d, "id = ?", deviceID).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListDevicesByProject(projectID uint) ([]model.Device, error) {
	var devices []model.Device
	err := s.db.Where("project_id = ?", projectID).Find(&devices).Error
	return devices, err
}

func (s *Store) ListDevicesByProjects(projectIDs []uint) ([]model.Device, error) {
	var devices []model.Device
	if len(projectIDs) == 0 {
		err := s.db.Find(&devices).Error
		return devices, err
	}
	err := s.db.Where("project_id IN ?", projectIDs).Find(&devices).Error
	return devices, err
}

// DevicePageFilter 设备分页检索条件（千台规模）。
// ProjectIDs 为租户作用域：nil=超管全部；单条 ProjectID 为项目内页；Keyword 同时模糊匹配 SN(id) 与名称；
// Status 取值 online/offline/disabled/pending。
type DevicePageFilter struct {
	ProjectIDs []uint
	ProjectID  uint
	Keyword    string
	Status     string
	Page       int
	PageSize   int
}

// DevicePageResult 分页结果。
type DevicePageResult struct {
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Items    []model.Device `json:"items"`
}

// buildDevicePageQuery 构造带租户作用域/项目/关键字/状态过滤的设备查询（不排序不分页），Count 与 Find 共用。
func (s *Store) buildDevicePageQuery(f DevicePageFilter) *gorm.DB {
	q := s.db.Model(&model.Device{})
	if len(f.ProjectIDs) > 0 {
		q = q.Where("project_id IN ?", f.ProjectIDs)
	}
	if f.ProjectID > 0 {
		q = q.Where("project_id = ?", f.ProjectID)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		// id 列为 utf8mb4_bin（大小写敏感），统一 LOWER 后再 LIKE；keyword 已转小写。
		q = q.Where("LOWER(id) LIKE ? OR LOWER(name) LIKE ?", like, like)
	}
	pendingCond := "product_id > 0 AND (device_secret = '' OR device_secret IS NULL)"
	switch f.Status {
	case "online":
		q = q.Where("enabled = ? AND status = ? AND NOT ("+pendingCond+")", true, model.DeviceStatusOnline)
	case "offline":
		q = q.Where("enabled = ? AND status <> ? AND NOT ("+pendingCond+")", true, model.DeviceStatusOnline)
	case "disabled":
		q = q.Where("enabled = ?", false)
	case "pending":
		q = q.Where(pendingCond)
	}
	return q
}

// ListDevicesPage 分页检索设备。默认排序：状态分组(在线 -> 离线 -> 待激活 -> 已禁用)
// -> 创建时间(created_at DESC) -> 最近活跃(last_active DESC，NULL 最后) -> SN。
// 先按状态四态聚组服务运维巡检直觉；同态内新建设备靠前，再按最新消息时间微调。
// 不开放任意排序字段（防注入）。
func (s *Store) ListDevicesPage(f DevicePageFilter) (*DevicePageResult, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 {
		f.PageSize = 24
	}
	q := s.buildDevicePageQuery(f)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}

	// 状态聚组：在线(0) < 离线(1, 兜底分支) < 待激活(2) < 已禁用(3)。
	// 待激活设备 enabled 恒为 1，不能再用 enabled DESC 置顶。
	// 待激活口径须与 buildDevicePageQuery 的 pending 过滤保持一致。
	pendingCond := "product_id > 0 AND (device_secret = '' OR device_secret IS NULL)"
	statusGroup := fmt.Sprintf(`CASE
			WHEN enabled = 1 AND status = %d AND NOT (%s) THEN 0
			WHEN enabled = 1 AND (%s) THEN 2
			WHEN enabled = 0 THEN 3
			ELSE 1
		END`,
		model.DeviceStatusOnline, pendingCond, pendingCond)
	var items []model.Device
	err := s.buildDevicePageQuery(f).
		Order(statusGroup).
		Order("created_at DESC").
		Order("last_active IS NULL").
		Order("last_active DESC").
		Order("id ASC").
		Limit(f.PageSize).
		Offset((f.Page - 1) * f.PageSize).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return &DevicePageResult{Total: total, Page: f.Page, PageSize: f.PageSize, Items: items}, nil
}

// ProjectDeviceCount 项目文件夹卡片上的设备计数角标。
type ProjectDeviceCount struct {
	ProjectID uint  `gorm:"column:project_id" json:"project_id"`
	Total     int64 `gorm:"column:total" json:"total"`
	Online    int64 `gorm:"column:online" json:"online"`
	Disabled  int64 `gorm:"column:disabled" json:"disabled"`
	Pending   int64 `gorm:"column:pending" json:"pending"`
}

// AggregateDeviceCountsByProjects 一条 GROUP BY 取各项目设备计数。
// projectIDs 为空（超管）时聚合全部项目；无设备的项目不出现在结果中，由前端补零。
func (s *Store) AggregateDeviceCountsByProjects(projectIDs []uint) ([]ProjectDeviceCount, error) {
	var rows []ProjectDeviceCount
	q := s.db.Model(&model.Device{}).
		Select(`project_id,
			COUNT(*) AS total,
			SUM(CASE WHEN enabled = 1 AND status = ? AND NOT (product_id > 0 AND (device_secret = '' OR device_secret IS NULL)) THEN 1 ELSE 0 END) AS online,
			SUM(CASE WHEN enabled = 0 THEN 1 ELSE 0 END) AS disabled,
			SUM(CASE WHEN product_id > 0 AND (device_secret = '' OR device_secret IS NULL) THEN 1 ELSE 0 END) AS pending`,
			model.DeviceStatusOnline).
		Group("project_id")
	if len(projectIDs) > 0 {
		q = q.Where("project_id IN ?", projectIDs)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) UpdateDeviceStatus(deviceID string, status int) error {
	return s.db.Model(&model.Device{}).Where("id = ?", deviceID).Update("status", status).Error
}

// UpdateDevice 更新设备可编辑属性（名称/所属项目/在线判定配置）。
// 注意：status 为运行时状态（MQTT 事件/超时扫描维护），绝不允许由此接口写入。
func (s *Store) UpdateDevice(deviceID string, name string, projectID uint, onlineMode string, offlineTimeoutSec int) error {
	return s.db.Model(&model.Device{}).Where("id = ?", deviceID).Updates(map[string]interface{}{
		"name":                name,
		"project_id":          projectID,
		"online_mode":         onlineMode,
		"offline_timeout_sec": offlineTimeoutSec,
	}).Error
}

// DeleteDevice 物理删除设备及其全部从属数据（Unscoped）。
// 设备主键即用户填写的字符串 ID，软删除会残留行导致同 ID 重建撞主键 1062，
// 因此与删租户/项目一致采用物理删除，删除后该 ID 立即可复用。每步均检查错误，杜绝静默吞错。
func (s *Store) DeleteDevice(deviceID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if r := tx.Unscoped().Where("device_id = ?", deviceID).Delete(&model.DeviceTag{}); r.Error != nil {
			return r.Error
		}
		if r := tx.Unscoped().Where("device_id = ?", deviceID).Delete(&model.DeviceData{}); r.Error != nil {
			return r.Error
		}
		if r := tx.Unscoped().Where("device_id = ?", deviceID).Delete(&model.ControlLog{}); r.Error != nil {
			return r.Error
		}
		if r := tx.Unscoped().Where("device_id = ?", deviceID).Delete(&model.OTALog{}); r.Error != nil {
			return r.Error
		}
		// 设备间消息/授权：任一端（发/收）命中即清
		if r := tx.Unscoped().Where("from_device_id = ? OR to_device_id = ?", deviceID, deviceID).Delete(&model.DeviceMessage{}); r.Error != nil {
			return r.Error
		}
		if r := tx.Unscoped().Where("from_device_id = ? OR to_device_id = ?", deviceID, deviceID).Delete(&model.DevicePeerAllow{}); r.Error != nil {
			return r.Error
		}
		// 设备维度的自定义控制指令（device_id 留空的项目通用指令不在此列）
		if r := tx.Unscoped().Where("device_id = ?", deviceID).Delete(&model.ControlCommand{}); r.Error != nil {
			return r.Error
		}
		// 设备维度的 OTA 任务
		if r := tx.Unscoped().Where("target_type = ? AND target_id = ?", "device", deviceID).Delete(&model.OTATask{}); r.Error != nil {
			return r.Error
		}
		return tx.Unscoped().Delete(&model.Device{}, "id = ?", deviceID).Error
	})
}

// DeviceTag

func (s *Store) CreateDeviceTag(t *model.DeviceTag) error {
	return s.db.Create(t).Error
}

func (s *Store) ListDeviceTags(deviceID string) ([]model.DeviceTag, error) {
	var tags []model.DeviceTag
	err := s.db.Where("device_id = ?", deviceID).Find(&tags).Error
	return tags, err
}

func (s *Store) DeleteDeviceTag(id uint) error {
	return s.db.Delete(&model.DeviceTag{}, id).Error
}

// ProjectTag

func (s *Store) CreateProjectTag(t *model.ProjectTag) error {
	return s.db.Create(t).Error
}

func (s *Store) ListProjectTags(projectID uint) ([]model.ProjectTag, error) {
	var tags []model.ProjectTag
	err := s.db.Where("project_id = ?", projectID).Find(&tags).Error
	return tags, err
}

func (s *Store) DeleteProjectTag(id uint) error {
	return s.db.Delete(&model.ProjectTag{}, id).Error
}

// DeviceData

func (s *Store) SaveDeviceData(d *model.DeviceData) error {
	return s.db.Create(d).Error
}

func (s *Store) ListDeviceData(deviceID string, limit int, offset int) ([]model.DeviceData, int64, error) {
	var total int64
	s.db.Model(&model.DeviceData{}).Where("device_id = ?", deviceID).Count(&total)
	var data []model.DeviceData
	err := s.db.Where("device_id = ?", deviceID).Order("ts desc").Limit(limit).Offset(offset).Find(&data).Error
	return data, total, err
}

// ControlLog

func (s *Store) CreateControlLog(l *model.ControlLog) error {
	return s.db.Create(l).Error
}

func (s *Store) GetControlLogByMsgID(msgID string) (*model.ControlLog, error) {
	var l model.ControlLog
	err := s.db.First(&l, "msg_id = ?", msgID).Error
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Store) UpdateControlLogAck(msgID string, code int, msg string) error {
	return s.db.Model(&model.ControlLog{}).Where("msg_id = ?", msgID).Updates(map[string]interface{}{
		"ack_code": code,
		"ack_msg":  msg,
	}).Error
}

func (s *Store) ListControlLogs(deviceID string, limit int, offset int) ([]model.ControlLog, int64, error) {
	var total int64
	q := s.db.Model(&model.ControlLog{})
	if deviceID != "" {
		q = q.Where("device_id = ?", deviceID)
	}
	q.Count(&total)
	var logs []model.ControlLog
	err := q.Order("id desc").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, total, err
}

// Stats

type DashboardStats struct {
	TotalTenants    int64 `json:"total_tenants"`
	TotalProjects   int64 `json:"total_projects"`
	TotalDevices    int64 `json:"total_devices"`
	OnlineDevices   int64 `json:"online_devices"`
	DisabledDevices int64 `json:"disabled_devices"`
	PendingDevices  int64 `json:"pending_devices"`
	// 消息量（数据库计数，租户作用域）：24h 与全量
	MessagesIn24H  int64 `json:"messages_in_24h"`
	MessagesOut24H int64 `json:"messages_out_24h"`
	MessagesInAll  int64 `json:"messages_in_total_db"`
	MessagesOutAll int64 `json:"messages_out_total_db"`
}

func (s *Store) GetDashboardStats(tenantID *uint) (*DashboardStats, error) {
	var stats DashboardStats
	since := time.Now().Add(-24 * time.Hour)

	if tenantID == nil {
		s.db.Model(&model.Tenant{}).Count(&stats.TotalTenants)
		s.db.Model(&model.Project{}).Count(&stats.TotalProjects)
		s.db.Model(&model.Device{}).Count(&stats.TotalDevices)
		s.db.Model(&model.Device{}).Where("status = ?", model.DeviceStatusOnline).Count(&stats.OnlineDevices)
		s.db.Model(&model.Device{}).Where("enabled = ?", false).Count(&stats.DisabledDevices)
		s.db.Model(&model.Device{}).
			Where("product_id > 0 AND (device_secret = '' OR device_secret IS NULL)").
			Count(&stats.PendingDevices)
		s.db.Model(&model.DeviceData{}).Where("ts >= ?", since.UnixMilli()).Count(&stats.MessagesIn24H)
		s.db.Model(&model.ControlLog{}).Where("created_at >= ?", since).Count(&stats.MessagesOut24H)
		s.db.Model(&model.DeviceData{}).Count(&stats.MessagesInAll)
		s.db.Model(&model.ControlLog{}).Count(&stats.MessagesOutAll)
	} else {
		var projectIDs []uint
		s.db.Model(&model.Project{}).Where("tenant_id = ?", *tenantID).Pluck("id", &projectIDs)
		stats.TotalProjects = int64(len(projectIDs))
		if len(projectIDs) > 0 {
			var deviceIDs []string
			s.db.Model(&model.Device{}).Where("project_id IN ?", projectIDs).Pluck("id", &deviceIDs)
			s.db.Model(&model.Device{}).Where("project_id IN ?", projectIDs).Count(&stats.TotalDevices)
			s.db.Model(&model.Device{}).
				Where("project_id IN ? AND status = ?", projectIDs, model.DeviceStatusOnline).
				Count(&stats.OnlineDevices)
			s.db.Model(&model.Device{}).
				Where("project_id IN ? AND enabled = ?", projectIDs, false).
				Count(&stats.DisabledDevices)
			s.db.Model(&model.Device{}).
				Where("project_id IN ? AND product_id > 0 AND (device_secret = '' OR device_secret IS NULL)", projectIDs).
				Count(&stats.PendingDevices)
			if len(deviceIDs) > 0 {
				s.db.Model(&model.DeviceData{}).
					Where("device_id IN ? AND ts >= ?", deviceIDs, since.UnixMilli()).
					Count(&stats.MessagesIn24H)
				s.db.Model(&model.ControlLog{}).
					Where("device_id IN ? AND created_at >= ?", deviceIDs, since).
					Count(&stats.MessagesOut24H)
				s.db.Model(&model.DeviceData{}).Where("device_id IN ?", deviceIDs).Count(&stats.MessagesInAll)
				s.db.Model(&model.ControlLog{}).Where("device_id IN ?", deviceIDs).Count(&stats.MessagesOutAll)
			}
		}
	}
	return &stats, nil
}

func (s *Store) ListDevicesByProjectIDs(projectIDs []uint) ([]model.Device, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	var devices []model.Device
	err := s.db.Where("project_id IN ?", projectIDs).Find(&devices).Error
	return devices, err
}

func (s *Store) GetLatestDataForDevice(deviceID string) (*model.DeviceData, error) {
	var d model.DeviceData
	err := s.db.Where("device_id = ?", deviceID).Order("ts desc").First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Firmware

func (s *Store) CreateFirmware(fw *model.Firmware) error {
	return s.db.Create(fw).Error
}

func (s *Store) ListFirmwares() ([]model.Firmware, error) {
	var list []model.Firmware
	err := s.db.Order("id desc").Find(&list).Error
	return list, err
}

func (s *Store) GetFirmwareByID(id uint) (*model.Firmware, error) {
	var fw model.Firmware
	err := s.db.First(&fw, id).Error
	if err != nil {
		return nil, err
	}
	return &fw, nil
}

func (s *Store) GetFirmwareByVersion(version string) (*model.Firmware, error) {
	var fw model.Firmware
	err := s.db.Where("version = ?", version).First(&fw).Error
	if err != nil {
		return nil, err
	}
	return &fw, nil
}

func (s *Store) GetLatestFirmware() (*model.Firmware, error) {
	var fw model.Firmware
	err := s.db.Order("id desc").First(&fw).Error
	if err != nil {
		return nil, err
	}
	return &fw, nil
}

func (s *Store) DeleteFirmware(id uint) error {
	return s.db.Delete(&model.Firmware{}, id).Error
}

// OTATask

func (s *Store) CreateOTATask(t *model.OTATask) error {
	return s.db.Create(t).Error
}

func (s *Store) ListOTATasks() ([]model.OTATask, error) {
	var tasks []model.OTATask
	err := s.db.Order("id desc").Find(&tasks).Error
	return tasks, err
}

func (s *Store) UpdateOTATaskStatus(taskID uint, status string) error {
	return s.db.Model(&model.OTATask{}).Where("id = ?", taskID).Update("status", status).Error
}

func (s *Store) CheckAndCompleteOTATask(taskID uint) {
	var logs []model.OTALog
	s.db.Where("task_id = ?", taskID).Find(&logs)
	if len(logs) == 0 {
		return
	}
	allDone := true
	for _, l := range logs {
		if l.Status != model.OTALogStatusSuccess && l.Status != model.OTALogStatusFailed {
			allDone = false
			break
		}
	}
	if allDone {
		s.UpdateOTATaskStatus(taskID, model.OTATaskStatusDone)
	}
}

// OTALog

func (s *Store) CreateOTALog(log *model.OTALog) error {
	return s.db.Create(log).Error
}

func (s *Store) ListOTALogs(taskID uint) ([]model.OTALog, error) {
	// taskID=0 表示不按任务过滤，返回全部升级日志（管理端“全部日志”查询）。
	q := s.db.Model(&model.OTALog{})
	if taskID > 0 {
		q = q.Where("task_id = ?", taskID)
	}
	var logs []model.OTALog
	err := q.Order("id").Find(&logs).Error
	return logs, err
}

func (s *Store) UpdateOTALogStatus(deviceID string, taskID uint, status string, progress int, errMsg string) error {
	return s.db.Model(&model.OTALog{}).
		Where("device_id = ? AND task_id = ?", deviceID, taskID).
		Updates(map[string]interface{}{
			"status":    status,
			"progress":  progress,
			"error_msg": errMsg,
		}).Error
}

func uintsToStrings(ids []uint) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = strconv.FormatUint(uint64(id), 10)
	}
	return result
}

// 三层在线判定继承表达式（设备 -> 项目 -> 系统全局）。
// 历史 devices.online_mode 空串按 connection 处理的语义保留：项目层也为空时回退到 globalMode。
// 调用方需保证 globalMode 已在 config 加载时归一为 connection/report/ping。
const effectiveOnlineModeExpr = "COALESCE(NULLIF(NULLIF(d.online_mode, ''), 'default'), NULLIF(NULLIF(p.online_mode, ''), 'default'), ?)"

// 有效超时（秒）：设备 >0 优先，否则项目 >0，否则 NULL（交由全局超时分支）。
const effectiveTimeoutExpr = "CASE WHEN d.offline_timeout_sec > 0 THEN d.offline_timeout_sec WHEN p.offline_timeout_sec > 0 THEN p.offline_timeout_sec ELSE NULL END"

// MarkOfflineDevices 离线回收扫描（三层在线判定继承：设备 -> 项目 -> 系统全局）。
//   - report：超过有效超时未上报(last_active 陈旧)即判离线；
//   - ping ：超过有效超时未回应 ping(last_active 陈旧)即判离线（连接假死也能检出）；
//   - connection：纯事件驱动（断开即离线），扫描不触碰。
//
// 设备/项目均未设超时时沿用全局 timeout；全局 <=0 视为禁用超时，不回收。
func (s *Store) MarkOfflineDevices(globalMode string, globalTimeout time.Duration) error {
	joinClause := "devices d LEFT JOIN projects p ON p.id = d.project_id AND p.deleted_at IS NULL"
	onlineCond := "d.status = ? AND d.deleted_at IS NULL"

	// ① 设备或项目显式设置了超时：按各行有效秒数回收
	if err := s.db.
		Table(joinClause).
		Where(onlineCond, model.DeviceStatusOnline).
		Where(effectiveOnlineModeExpr+" IN ('report','ping')", globalMode).
		Where(effectiveTimeoutExpr+" IS NOT NULL").
		Where("d.last_active < DATE_SUB(?, INTERVAL "+effectiveTimeoutExpr+" SECOND)", time.Now()).
		Update("d.status", model.DeviceStatusOffline).Error; err != nil {
		return err
	}

	// ② 设备/项目均未设超时：沿用全局超时（全局禁用超时时跳过）
	if globalTimeout > 0 {
		if err := s.db.Table(joinClause).
			Where(onlineCond, model.DeviceStatusOnline).
			Where(effectiveOnlineModeExpr+" IN ('report','ping')", globalMode).
			Where(effectiveTimeoutExpr+" IS NULL").
			Where("d.last_active < ?", time.Now().Add(-globalTimeout)).
			Update("d.status", model.DeviceStatusOffline).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListPingModeDevices 返回有效判定模式（三层继承解析后）为 ping 且已启用的设备 ID（平台探活目标）。
// 包含当前离线设备：ping 到达且设备应答后可经 handlePingAck 恢复在线。
func (s *Store) ListPingModeDevices(globalMode string) ([]string, error) {
	var ids []string
	err := s.db.Table("devices d LEFT JOIN projects p ON p.id = d.project_id AND p.deleted_at IS NULL").
		Where("d.enabled = ? AND d.deleted_at IS NULL", true).
		Where(effectiveOnlineModeExpr+" = ?", globalMode, "ping").
		Pluck("d.id", &ids).Error
	return ids, err
}

// UpdateDeviceLastActive updates the device's last active time
func (s *Store) UpdateDeviceLastActive(deviceID string) error {
	return s.db.Model(&model.Device{}).
		Where("id = ?", deviceID).
		Update("last_active", time.Now()).Error
}

// UpdateDeviceFirstTs updates the device's first boot time
func (s *Store) UpdateDeviceFirstTs(deviceID string, firstTs int64) error {
	return s.db.Model(&model.Device{}).
		Where("id = ?", deviceID).
		Update("first_ts", firstTs).Error
}
