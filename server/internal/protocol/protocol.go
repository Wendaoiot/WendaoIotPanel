package protocol

import (
	"encoding/json"
	"regexp"
	"strings"
)

const (
	CodeSuccess             = 0
	CodeDeviceNotRegistered = 1
	CodeTagNotConfigured    = 2
	CodeParamError          = 3
	CodeAlreadyActivated    = 4 // 一型一密：设备已激活（一机一密已签发）
	CodeDeviceOffline       = 5 // D2D：目标设备离线
	CodeForbidden           = 6 // D2D：跨租户且无白名单授权
	CodePeerNotFound        = 7 // D2D：目标设备不存在
)

// ===================== 一型一密动态注册 =====================

// BootstrapSep 引导连接用户名分隔符：username = "{SN}&{ProductKey}"。
// '&' 不在 SN/ProductKey 允许字符集（[A-Za-z0-9_-]）内，与普通设备连接天然不歧义。
const BootstrapSep = "&"

var (
	SNPattern         = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	ProductKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]{8,40}$`)
)

// ParseBootstrapUsername 解析引导连接用户名 "SN&ProductKey"，两段均须符合各自字符规范。
func ParseBootstrapUsername(username string) (sn, productKey string, ok bool) {
	if !strings.Contains(username, BootstrapSep) {
		return "", "", false
	}
	parts := strings.SplitN(username, BootstrapSep, 2)
	sn, productKey = parts[0], parts[1]
	if !SNPattern.MatchString(sn) || !ProductKeyPattern.MatchString(productKey) {
		return "", "", false
	}
	return sn, productKey, true
}

// 引导连接仅可使用与其 SN 绑定的两个注册主题（ACL 特判）：
//
//	publish   wendao/register/{sn}/req
//	subscribe wendao/register/{sn}/reply
func TopicRegisterReq(sn string) string  { return "wendao/register/" + sn + "/req" }
func TopicRegisterResp(sn string) string { return "wendao/register/" + sn + "/reply" }
func TopicRegisterReqSub() string        { return "wendao/register/+/req" }

// RegisterReq 设备动态注册请求（引导连接发布到 wendao/register/{sn}/req）。
type RegisterReq struct {
	ID         string `json:"id"`          // 消息 id（设备生成，用于关联应答）
	SN         string `json:"sn"`          // 设备序列号（须等于引导用户名中的 SN）
	ProductKey string `json:"product_key"` // 产品凭证（须等于引导用户名中的 ProductKey）
	FwVersion  string `json:"fw_version,omitempty"`
}

// RegisterResp 平台动态注册应答（wendao/register/{sn}/reply）。
// code=0 时 device_secret 为新签发的一机一密（仅下发一次）；
// code=4 表示设备已激活，引导连接将被拒绝/断开，设备应改用一机一密重连。
type RegisterResp struct {
	ID           string `json:"id"`
	Code         int    `json:"code"`
	Msg          string `json:"msg"`
	DeviceSecret string `json:"device_secret,omitempty"`
}

type UplinkRequest struct {
	ID      string             `json:"id"`
	Ts      int64              `json:"ts"`
	Version string             `json:"version"`
	FirstTs int64              `json:"first_ts"`
	Tags    map[string]float64 `json:"tags"`
}

type UplinkResponse struct {
	ID   string `json:"id"`
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type DownlinkRequest struct {
	ID   string             `json:"id"`
	Ts   int64              `json:"ts"`
	Tags map[string]float64 `json:"tags"`
}

type DownlinkResponse struct {
	ID   string `json:"id"`
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func TopicData(deviceID string) string       { return "wendao/" + deviceID + "/data" }
func TopicDataAck(deviceID string) string    { return "wendao/" + deviceID + "/data/ack" }
func TopicControl(deviceID string) string    { return "wendao/" + deviceID + "/control" }
func TopicControlAck(deviceID string) string { return "wendao/" + deviceID + "/control/ack" }

// TopicControlAckResp 服务器对设备控制回执的响应主题：设备向 wendao/{id}/control/ack 回执行
// 结果后，服务器向该主题发布一条响应（DownlinkResponse），供端侧确认平台已收到回执。
func TopicControlAckResp(deviceID string) string { return "wendao/" + deviceID + "/control/ack/resp" }

// 在线判定 ping（ping 模式）：平台周期性向 wendao/{id}/ping 发布探活（DownlinkRequest{id,ts}），
// 设备须在订阅后向 wendao/{id}/ping/ack 回应 DownlinkResponse{id,code:0}；
// 平台超过 offline_timeout_sec 未收到应答即判离线。不进控制日志，QoS1。
func TopicPing(deviceID string) string    { return "wendao/" + deviceID + "/ping" }
func TopicPingAck(deviceID string) string { return "wendao/" + deviceID + "/ping/ack" }
func TopicPingAckSub() string             { return "wendao/+/ping/ack" }

func TopicDataSub() string                    { return "wendao/+/data" }
func TopicControlAckSub() string              { return "wendao/+/control/ack" }
func TopicOTA(deviceID string) string         { return "wendao/" + deviceID + "/ota" }
func TopicOTAProgress(deviceID string) string { return "wendao/" + deviceID + "/ota/progress" }
func TopicOTAAck(deviceID string) string      { return "wendao/" + deviceID + "/ota/ack" }
func TopicOTAProgressSub() string             { return "wendao/+/ota/progress" }
func TopicOTAAckSub() string                  { return "wendao/+/ota/ack" }

// D2D：设备发布 wendao/{id}/peer 请求投递；订阅 wendao/{id}/peer/ack 收应答；
// 订阅 wendao/{id}/inbox 接收其他设备（经平台投递）的消息。
func TopicPeer(deviceID string) string    { return "wendao/" + deviceID + "/peer" }
func TopicPeerAck(deviceID string) string { return "wendao/" + deviceID + "/peer/ack" }
func TopicInbox(deviceID string) string   { return "wendao/" + deviceID + "/inbox" }
func TopicPeerSub() string                { return "wendao/+/peer" }

// TopicKicked 服务器主动断开（互踢等）前向设备发布的通知主题。
// 设备订阅 wendao/{id}/kicked 可得知断开原因；QoS1 消息会进入被踢会话的持久化队列，
// clean session=false 的客户端重连后仍能收到。
func TopicKicked(deviceID string) string { return "wendao/" + deviceID + "/kicked" }

type OTARequest struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Version string `json:"version"`
	URL     string `json:"url"`
	MD5     string `json:"md5"`
	Size    int64  `json:"size"`
}

// ===================== 设备间通信（D2D） =====================

// PeerMessage 设备发给平台的消息（wendao/{id}/peer）：要求平台投递给 To 指定的目标。
//
// To 取值语义：
//   - "dev_001"         ：点对点投递（设备 ID，大小写敏感；唯一支持跨租户白名单的模式）
//   - "dev_001,dev_002" ：多播（逗号分隔设备 ID；仅限发送方同租户）
//   - "*"               ：租户级广播——发送方租户内全部在线启用设备（跨项目，不含发送方自己）
//   - "project:12"      ：项目级广播——项目 12（须属发送方租户）内全部在线启用设备
//
// 广播/多播只投在线设备、不做离线暂存，ack 携带 "delivered 成功数/总数" 汇总结果。
type PeerMessage struct {
	ID      string          `json:"id"`      // 消息 id（设备生成，用于 ack 关联）
	Ts      int64           `json:"ts"`      // 设备时间戳（毫秒，仅参考）
	To      string          `json:"to"`      // 投递目标，取值语义见上方注释
	Type    string          `json:"type"`    // 业务类型：notify / cmd / event / 自定义
	Payload json.RawMessage `json:"payload"` // 透传 JSON（对象或任意 JSON 值）
}

// PeerAck 平台对 PeerMessage 的应答（wendao/{id}/peer/ack）。
type PeerAck struct {
	ID   string `json:"id"`
	Code int    `json:"code"` // 0=已投递 5=目标离线 6=越权 7=目标不存在
	Msg  string `json:"msg"`
}

// InboxMessage 平台投递到设备收件箱（wendao/{id}/inbox）的信封。
type InboxMessage struct {
	ID      string          `json:"id"`      // 原消息 id
	From    string          `json:"from"`    // 发送方设备 ID
	Type    string          `json:"type"`    // 业务类型
	Payload json.RawMessage `json:"payload"` // 透传 JSON
	Ts      int64           `json:"ts"`      // 平台投递时间（毫秒）
}

type OTAProgress struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Progress int    `json:"progress"`
	Status   string `json:"status"` // downloading/installing
}

type OTAResponse struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// KickedNotice 服务器主动断开连接前发布的通知（wendao/{id}/kicked）。
type KickedNotice struct {
	Reason string `json:"reason"` // new_login=同设备新连接登录（互踢）；disabled=设备被禁用
	Msg    string `json:"msg"`    // 人类可读说明
	Ts     int64  `json:"ts"`     // 服务器时间（毫秒）
}
