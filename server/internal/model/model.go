package model

import (
	"time"

	"gorm.io/gorm"
)

type Tenant struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// 唯一性为 逻辑名+软删标记 组合：软删行不再阻塞同名新租户（修复 1062 复发）
	Name      string         `gorm:"type:varchar(100);uniqueIndex:uk_tenant_name_alive" json:"name"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"uniqueIndex:uk_tenant_name_alive" json:"-"`
}

type Project struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	TenantID uint   `gorm:"index;uniqueIndex:uk_proj_tenant_name" json:"tenant_id"`
	Name     string `gorm:"type:varchar(100);uniqueIndex:uk_proj_tenant_name" json:"name"`
	// 项目级在线判定默认（设备未显式覆盖时生效；空值/0 表示沿用系统 config 默认）：
	//   OnlineMode        — '' / connection / report / ping
	//   OfflineTimeoutSec — 0=沿用系统 offline_timeout_sec
	OnlineMode        string         `gorm:"type:varchar(20);default:''" json:"online_mode"`
	OfflineTimeoutSec int            `gorm:"default:0" json:"offline_timeout_sec"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// Product 产品（一型一密）：同型号设备烧录同一套 ProductKey/ProductSecret，
// 首次 mqtts 连接时动态注册换取一机一密。产品为租户级实体。
type Product struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	ProductKey    string `gorm:"type:varchar(40) COLLATE utf8mb4_bin;uniqueIndex" json:"product_key"`
	ProductSecret string `gorm:"type:varchar(128)" json:"-"` // bcrypt 哈希，明文仅创建/重置时返回一次
	TenantID      uint   `gorm:"index" json:"tenant_id"`
	Name          string `gorm:"type:varchar(100)" json:"name"`
	// DynRegEnabled 产品级动态注册开关：关闭后引导连接直接拒绝
	DynRegEnabled bool           `gorm:"default:true" json:"dyn_reg_enabled"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

const (
	DeviceStatusOffline  = 0
	DeviceStatusOnline   = 1
	DeviceStatusInactive = 2
)

type Device struct {
	// COLLATE utf8mb4_bin：设备 ID 严格区分大小写（与 MQTT username/topic/ACL 语义一致）。
	// 存量库（devices.id 原为库默认 utf8mb4_unicode_ci）需一次性迁移：
	// 对 devices.id 及 device_data/device_tags/control_logs/control_commands/ota_logs.device_id
	// 执行 ALTER TABLE ... MODIFY ... VARCHAR(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL。
	// 迁移前先 SELECT LOWER(id), COUNT(*) ... GROUP BY LOWER(id) HAVING COUNT(*)>1 排查仅大小写差异的重名设备。
	ID           string `gorm:"primaryKey;type:varchar(100) COLLATE utf8mb4_bin" json:"id"`
	ProjectID    uint   `gorm:"index" json:"project_id"`
	TenantID     uint   `gorm:"index" json:"tenant_id"` // 冗余租户，便于作用域过滤
	Name         string `gorm:"type:varchar(100)" json:"name"`
	Status       int    `gorm:"default:0" json:"status"`
	Enabled      bool   `gorm:"default:true" json:"enabled"` // false 等价于禁用(Inactive)：拒绝上报/控制
	DeviceSecret string `gorm:"type:varchar(128)" json:"-"`  // 一机一密，接入 broker 认证
	// 一型一密动态注册：
	//   ProductID > 0 且 DeviceSecret 为空 = 已预录待激活（仅允许引导注册连接）；
	//   激活写入一机一密后置 ActivatedAt；ProductID = 0 为传统手工创建设备。
	ProductID   uint       `gorm:"index" json:"product_id"`
	ActivatedAt *time.Time `json:"activated_at"`
	// 设备级在线判定（系统默认 connection；历史空串等同 connection）：
	//   connection — 按 MQTT 连接/断开事件实时判定
	//   report     — 按上报超时判定（连接事件仍置在线，超时未上报由扫描回收）
	//   ping       — 按探活应答超时判定
	OnlineMode        string         `gorm:"type:varchar(20);default:''" json:"online_mode"`
	OfflineTimeoutSec int            `gorm:"default:0" json:"offline_timeout_sec"` // 0=沿用全局 offline_timeout_sec
	FirstTs           int64          `gorm:"default:0" json:"first_ts"`
	LastActive        *time.Time     `gorm:"index" json:"last_active"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

type DeviceTag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	DeviceID  string    `gorm:"index;type:varchar(100)" json:"device_id"`
	TagKey    string    `gorm:"type:varchar(50)" json:"tag_key"`
	Name      string    `gorm:"type:varchar(100)" json:"name"` // 备注名/显示名（用户可填）
	Unit      string    `gorm:"type:varchar(20)" json:"unit"`  // 单位
	Interface string    `gorm:"type:varchar(50)" json:"interface"`
	Formula   string    `gorm:"type:varchar(500)" json:"formula"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectTag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"index" json:"project_id"`
	TagKey    string    `gorm:"type:varchar(50)" json:"tag_key"`
	TagName   string    `gorm:"type:varchar(100)" json:"tag_name"`
	Unit      string    `gorm:"type:varchar(20)" json:"unit"`
	DataType  string    `gorm:"type:varchar(20);default:'number'" json:"data_type"`
	Writable  bool      `gorm:"default:false" json:"writable"`
	CreatedAt time.Time `json:"created_at"`
}

type DeviceData struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	DeviceID  string    `gorm:"index;type:varchar(100)" json:"device_id"`
	MsgID     string    `gorm:"type:varchar(100)" json:"msg_id"`
	Ts        int64     `gorm:"index" json:"ts"` // 权威时间戳：服务端接收时间（Unix 毫秒），用于排序/实时/图表
	DeviceTs  int64     `json:"device_ts"`       // 设备自报时间戳（毫秒），仅参考，不参与排序
	Version   string    `gorm:"type:varchar(50)" json:"version"`
	Data      string    `gorm:"type:text" json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

type ControlLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	DeviceID  string    `gorm:"index;type:varchar(100)" json:"device_id"`
	TenantID  uint      `gorm:"index" json:"tenant_id,omitempty"`
	MsgID     string    `gorm:"index;type:varchar(100)" json:"msg_id"`
	Tags      string    `gorm:"type:text" json:"tags"`
	Status    string    `gorm:"type:varchar(20);default:'pending'" json:"status"` // pending/delivered/success/failed/timeout
	AckCode   *int      `json:"ack_code"`
	AckMsg    string    `json:"ack_msg"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 控制指令状态
const (
	ControlStatusPending   = "pending"
	ControlStatusDelivered = "delivered"
	ControlStatusSuccess   = "success"
	ControlStatusFailed    = "failed"
	ControlStatusTimeout   = "timeout"
)

// ControlCommand 是用户可自定义命名/取值的控制按钮（项目维度）。
type ControlCommand struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"index" json:"project_id"`
	DeviceID  string    `gorm:"type:varchar(100)" json:"device_id,omitempty"` // 留空表示项目通用
	Name      string    `gorm:"type:varchar(100)" json:"name"`                // 按钮显示名（用户命名）
	TagKey    string    `gorm:"type:varchar(50)" json:"tag_key"`
	Value     float64   `json:"value"`
	Icon      string    `gorm:"type:varchar(50)" json:"icon"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Danger    bool      `gorm:"default:false" json:"danger"` // 高危指令（重启/出厂），需二次确认
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	RoleSuperAdmin  = "super_admin"
	RoleTenantAdmin = "tenant_admin"
)

type AdminUser struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Username      string     `gorm:"type:varchar(50);uniqueIndex" json:"username"`
	Password      string     `gorm:"type:varchar(200)" json:"-"`
	Role          string     `gorm:"type:varchar(20)" json:"role"`
	TenantID      *uint      `json:"tenant_id"`
	TokenVersion  uint       `gorm:"default:0" json:"-"` // 改密/重置/删除后递增，使旧 token 失效
	PassChangedAt *time.Time `json:"-"`                  // 可空：未改过密码时为 NULL（MySQL8 严格模式不接受零值日期）
	// UIOptions 每账号界面偏好（JSON），如设备管理是否按项目二级浏览。json:"-" 不随用户对象返回，走独立偏好接口。
	UIOptions string    `gorm:"type:text" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Firmware struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(100)" json:"name"`
	Version     string    `gorm:"type:varchar(50);index" json:"version"`
	URL         string    `gorm:"type:varchar(500)" json:"url"`
	Size        int64     `json:"size"`
	MD5         string    `gorm:"type:varchar(64)" json:"md5"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	OTATaskStatusPending = "pending"
	OTATaskStatusRunning = "running"
	OTATaskStatusDone    = "done"
)

// ===================== 设备间通信（D2D） =====================

// DeviceMessageStatus 设备间消息投递结果
const (
	DeviceMsgDelivered        = "delivered"         // 已投递到目标设备 inbox
	DeviceMsgRecipientOffline = "recipient_offline" // 目标设备离线，拒绝投递
	DeviceMsgRejected         = "rejected"          // 越权（跨租户且无白名单）
	DeviceMsgTargetNotFound   = "target_not_found"  // 目标设备不存在或未注册
)

// DeviceMessage 设备间消息留痕（设备 A → 平台 → 设备 B 的 inbox）。
type DeviceMessage struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	MsgID        string    `gorm:"type:varchar(100);index" json:"msg_id"`         // 设备上报的消息 id
	FromDeviceID string    `gorm:"type:varchar(100);index" json:"from_device_id"` // 发送方设备 ID
	ToDeviceID   string    `gorm:"type:varchar(100);index" json:"to_device_id"`   // 接收方设备 ID
	TenantID     uint      `gorm:"index" json:"tenant_id"`                        // 发送方所属租户
	Type         string    `gorm:"type:varchar(50)" json:"type"`                  // 业务类型（notify/cmd/event…）
	Payload      string    `gorm:"type:text" json:"payload"`                      // JSON 透传内容
	Status       string    `gorm:"type:varchar(30);index" json:"status"`          // delivered/offline/rejected/not_found
	Ts           int64     `json:"ts"`                                            // 服务端接收时间（毫秒）
	CreatedAt    time.Time `json:"created_at"`
}

// DevicePeerAllow 跨租户设备间通信白名单（from → to 单向授权）。
type DevicePeerAllow struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	FromDeviceID string    `gorm:"type:varchar(100);uniqueIndex:uk_peer_from_to" json:"from_device_id"`
	ToDeviceID   string    `gorm:"type:varchar(100);uniqueIndex:uk_peer_from_to" json:"to_device_id"`
	TenantID     uint      `gorm:"index" json:"tenant_id"` // 发起方租户（管理方便）
	Remark       string    `gorm:"type:varchar(200)" json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
}

type OTATask struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	FirmwareID uint      `json:"firmware_id"`
	TargetType string    `gorm:"type:varchar(20)" json:"target_type"` // device/project
	TargetID   string    `gorm:"type:varchar(100)" json:"target_id"`
	Status     string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

const (
	OTALogStatusPending     = "pending"
	OTALogStatusDownloading = "downloading"
	OTALogStatusInstalling  = "installing"
	OTALogStatusSuccess     = "success"
	OTALogStatusFailed      = "failed"
)

type OTALog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"index" json:"task_id"`
	DeviceID  string    `gorm:"type:varchar(100);index" json:"device_id"`
	Status    string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	Progress  int       `gorm:"default:0" json:"progress"`
	ErrorMsg  string    `gorm:"type:varchar(500)" json:"error_msg"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
