package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"wendaoiotpanel/internal/evaluate"
	"wendaoiotpanel/internal/events"
	"wendaoiotpanel/internal/metrics"
	"wendaoiotpanel/internal/model"
	"wendaoiotpanel/internal/protocol"
	"wendaoiotpanel/internal/store"
	cryptopkg "wendaoiotpanel/pkg/crypto"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// dynRegKickGrace 动态注册应答入队到踢掉引导连接之间的投递宽限
// （QoS1 消息下发 + 设备端处理时间；clean session 设备唯一接收窗口）。
var dynRegKickGrace = 500 * time.Millisecond

// SessionKicker 互踢能力（由独立 broker 经内部 HTTP 实现，main 启动时注入）。
// username 语义：设备ID / 引导用户名 "SN&ProductKey"；excludeClientID 排除新连接自身。
type SessionKicker interface {
	KickByUsername(username, excludeClientID string) ([]string, error)
}

type Client struct {
	client mqtt.Client
	store  *store.Store
	bus    *events.Bus
	cfg    Config
	kicker SessionKicker // 互踢实现（main 注入独立 broker 的内部 HTTP 客户端）；nil 时静默禁用
	mu     sync.RWMutex

	// initialConns 计数 CONNECT 成功次数：首次连接由 main 显式 Subscribe 负责，
	// OnConnect 跳过以避免与显式订阅同一组主题并发（曾导致 subscribe token 永不完成）；
	// 之后的断线重连才在 OnConnect 中自动 resubscribe。
	initialConns int
}

func New(cfg Config, s *store.Store, bus *events.Bus) (*Client, error) {
	c := &Client{store: s, cfg: cfg, bus: bus}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetResumeSubs(true).
		SetCleanSession(false).
		SetOnConnectHandler(func(_ mqtt.Client) {
			// 加锁计数：仅第 2 次及以后的连接（断线重连）自动补订阅；
			// 首次连接的订阅由 main 的 Subscribe 同步负责，避免并发重复订阅。
			c.mu.Lock()
			c.initialConns++
			isReconnect := c.initialConns > 1
			c.mu.Unlock()
			if isReconnect {
				c.resubscribe()
			}
		})

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	c.client = mqtt.NewClient(opts)
	return c, nil
}

func (c *Client) Connect() error {
	token := c.client.Connect()
	token.Wait()
	return token.Error()
}

func (c *Client) Subscribe() error {
	topics := map[string]byte{
		protocol.TopicDataSub():        1,
		protocol.TopicControlAckSub():  1,
		protocol.TopicPingAckSub():     1,
		protocol.TopicOTAProgressSub(): 1,
		protocol.TopicOTAAckSub():      1,
		protocol.TopicPeerSub():        1,
		protocol.TopicRegisterReqSub(): 1,
	}
	token := c.client.SubscribeMultiple(topics, c.onMessage)
	token.Wait()
	return token.Error()
}

// SetKicker 注入互踢实现（main 启动时调用，传独立 broker 的内部 HTTP 客户端）。
// 未注入时 KickDeviceSessions/kickBootstrap 静默跳过。
func (c *Client) SetKicker(k SessionKicker) {
	c.kicker = k
}

// KickDeviceSessions 踢掉某设备当前的在线会话（互踢入口），委托独立 broker。
// excludeClientID 为新连接自身，须排除；传空踢全部。
func (c *Client) KickDeviceSessions(deviceID, excludeClientID string) ([]string, error) {
	if c.kicker == nil {
		return nil, nil
	}
	return c.kicker.KickByUsername(deviceID, excludeClientID)
}

// NotifyKicked 主动断开某设备前，向 wendao/{id}/kicked 发布断开原因（QoS1）。
// 同步等待 broker 确认入队后再返回：调用方随后踢线时，通知已具备投递窗口。
// QoS1 消息会进入被踢会话的持久化队列，客户端重连（clean session=false）后可收到；
// clean session=true 的在线会话依赖调用方预留的投递宽限时间，极端情况下仍可能丢失。
func (c *Client) NotifyKicked(deviceID, reason, msg string) {
	notice, _ := json.Marshal(protocol.KickedNotice{Reason: reason, Msg: msg, Ts: time.Now().UnixMilli()})
	token := c.client.Publish(protocol.TopicKicked(deviceID), 1, false, notice)
	token.Wait()
	if err := token.Error(); err != nil {
		log.Printf("mqtt: publish kicked notice to %s failed: %v", deviceID, err)
	}
}

func (c *Client) resubscribe() {
	topics := map[string]byte{
		protocol.TopicDataSub():        1,
		protocol.TopicControlAckSub():  1,
		protocol.TopicPingAckSub():     1,
		protocol.TopicOTAProgressSub(): 1,
		protocol.TopicOTAAckSub():      1,
		protocol.TopicPeerSub():        1,
		protocol.TopicRegisterReqSub(): 1,
	}
	c.client.SubscribeMultiple(topics, c.onMessage)
}

func extractDeviceID(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func (c *Client) onMessage(_ mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()

	deviceID := extractDeviceID(topic)

	// 一型一密动态注册：wendao/register/{sn}/req
	if deviceID == "register" && strings.HasPrefix(topic, "wendao/register/") &&
		strings.HasSuffix(topic, "/req") {
		c.handleDynRegister(topic, msg.Payload())
		return
	}

	if strings.HasSuffix(topic, "/ota/progress") {
		c.handleOTAProgress(deviceID, msg.Payload())
	} else if strings.HasSuffix(topic, "/ota/ack") {
		c.handleOTAAck(deviceID, msg.Payload())
	} else if strings.HasSuffix(topic, "/peer") {
		c.handlePeer(deviceID, msg.Payload())
	} else if strings.HasSuffix(topic, "/data") {
		c.handleUplink(deviceID, msg.Payload())
	} else if strings.HasSuffix(topic, "/control/ack") {
		c.handleControlAck(deviceID, msg.Payload())
	} else if strings.HasSuffix(topic, "/ping/ack") {
		c.handlePingAck(deviceID, msg.Payload())
	}
}

func (c *Client) handleUplink(deviceID string, payload []byte) {
	var req protocol.UplinkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("mqtt uplink: invalid JSON from %s: %v (payload: %.200s)", deviceID, err, payload)
		return
	}

	var resp protocol.UplinkResponse
	resp.ID = req.ID

	dev, err := c.store.GetDevice(deviceID)
	if err != nil {
		resp.Code = protocol.CodeDeviceNotRegistered
		resp.Msg = "device not registered"
		c.publishAck(protocol.TopicDataAck(deviceID), resp)
		return
	}

	if !dev.Enabled {
		resp.Code = protocol.CodeDeviceNotRegistered
		resp.Msg = "device disabled"
		c.publishAck(protocol.TopicDataAck(deviceID), resp)
		return
	}

	// 上线：仅对启用设备刷新状态/活跃时间；回填冗余租户字段。
	if err := c.store.SetDeviceRuntimeStatus(deviceID, true); err != nil {
		log.Printf("mqtt uplink: set runtime status error: %v", err)
	}
	if dev.TenantID == 0 {
		if proj, perr := c.store.GetProjectByID(dev.ProjectID); perr == nil {
			_ = c.store.SetDeviceTenant(deviceID, proj.TenantID)
			dev.TenantID = proj.TenantID
		}
	}

	// Save first boot time (updated on each device reboot)
	if req.FirstTs > 0 {
		// FirstTs 由设备上报，仅在合理范围采信（毫秒/秒归一为毫秒）
		firstTs := normalizeMillis(req.FirstTs)
		if err := c.store.UpdateDeviceFirstTs(deviceID, firstTs); err != nil {
			log.Printf("mqtt uplink: update first_ts error: %v", err)
		}
	}

	deviceTags, _ := c.store.ListDeviceTags(deviceID)
	tagMap := make(map[string]model.DeviceTag)
	for _, dt := range deviceTags {
		tagMap[dt.TagKey] = dt
	}

	computed := make(map[string]float64)
	for k, v := range req.Tags {
		computed[k] = v
		if dt, ok := tagMap[k]; ok && dt.Formula != "" {
			if cv, err := evaluate.Eval(dt.Formula, v); err == nil {
				computed[k] = cv
			}
		}
	}

	dataJSON, _ := json.Marshal(computed)
	now := time.Now().UnixMilli()
	dd := &model.DeviceData{
		DeviceID: deviceID,
		MsgID:    req.ID,
		Ts:       now,                     // 权威时间：服务端接收时间（毫秒）
		DeviceTs: normalizeMillis(req.Ts), // 设备自报时间戳仅参考
		Version:  req.Version,
		Data:     string(dataJSON),
	}
	if err := c.store.SaveDeviceData(dd); err != nil {
		log.Printf("mqtt uplink: save device data error: %v", err)
	}

	// 仪表盘消息流入实时计数（保存成功即计入）
	metrics.AddIn(dev.TenantID)

	resp.Code = protocol.CodeSuccess
	resp.Msg = "ok"
	c.publishAck(protocol.TopicDataAck(deviceID), resp)

	// 实时推送给前端（按租户）
	if c.bus != nil && dev.TenantID != 0 {
		c.bus.BroadcastToTenant(dev.TenantID, events.Message{
			Type: "device_data",
			Data: map[string]interface{}{
				"device_id": deviceID,
				"ts":        now,
				"tags":      computed,
			},
		})
	}
}

// handleControlAck 设备控制回执：wendao/{id}/control/ack → 更新控制日志状态机 →
// 前端 WS 广播 → 向 wendao/{id}/control/ack/resp 发布服务器响应（端侧可订阅确认平台已收到）。
// 响应码：0=已受理（含重复 ack 幂等）；3=msg id 不存在；非法 JSON 不回（与 data/peer 一致）。
func (c *Client) handleControlAck(deviceID string, payload []byte) {
	var ack protocol.DownlinkResponse
	if err := json.Unmarshal(payload, &ack); err != nil {
		log.Printf("mqtt control ack: invalid JSON from %s: %v (payload: %.200s)", deviceID, err, payload)
		return
	}

	resp := protocol.DownlinkResponse{ID: ack.ID}
	if ack.ID == "" {
		resp.Code, resp.Msg = protocol.CodeParamError, "id required"
		c.publishAck(protocol.TopicControlAckResp(deviceID), resp)
		return
	}
	if err := c.store.ApplyControlAck(ack.ID, ack.Code, ack.Msg); err != nil {
		log.Printf("mqtt control ack: apply error: %v", err)
		resp.Code, resp.Msg = protocol.CodeParamError, "unknown msg id"
		c.publishAck(protocol.TopicControlAckResp(deviceID), resp)
		return
	}
	resp.Code, resp.Msg = protocol.CodeSuccess, "ok"
	c.publishAck(protocol.TopicControlAckResp(deviceID), resp)

	clog, err := c.store.GetControlLogByMsgID(ack.ID)
	if err != nil {
		return
	}
	if c.bus != nil && clog.TenantID != 0 {
		status := model.ControlStatusSuccess
		if ack.Code != 0 {
			status = model.ControlStatusFailed
		}
		c.bus.BroadcastToTenant(clog.TenantID, events.Message{
			Type: "control_ack",
			Data: map[string]interface{}{
				"device_id": clog.DeviceID,
				"msg_id":    ack.ID,
				"code":      ack.Code,
				"msg":       ack.Msg,
				"status":    status,
			},
		})
	}
}

// normalizeMillis 将设备上报时间戳归一为毫秒：秒级（< 1e12）×1000。
func normalizeMillis(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	if ts < 1_000_000_000_000 {
		return ts * 1000
	}
	return ts
}

// handlePeer 设备间消息：设备发布 wendao/{from}/peer → 解析目标 → 校验 → 投递目标 inbox →
// 回汇总 ack → 逐目标留痕。
//
// to 目标语义（见 protocol.PeerMessage）：
//   - 单设备 ID：点对点，唯一支持跨租户白名单（DevicePeerAllow）的模式
//   - "a,b,c" 多播 / "*" 租户级广播 / "project:{id}" 项目级广播：仅限发送方同租户
//
// 授权规则：
//   - 双方同租户：默认允许
//   - 跨租户（仅单目标）：必须存在白名单 DevicePeerAllow(from → to)，否则 code=6
//
// 结果码：0=全部/部分投递成功（msg 带 "delivered 成功数/总数"）；5=目标离线；6=越权；7=目标不存在。
// 广播/多播只投在线启用设备，离线目标计入总数但不计成功，不做离线暂存。
const maxPeerTargets = 500

func (c *Client) handlePeer(fromID string, payload []byte) {
	var req protocol.PeerMessage
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("mqtt peer: invalid JSON from %s: %v (payload: %.200s)", fromID, err, payload)
		return
	}

	ack := protocol.PeerAck{ID: req.ID}

	recordPeer := func(toID, status string) {
		var tenantID uint
		if dev, err := c.store.GetDevice(fromID); err == nil {
			tenantID = dev.TenantID
		}
		if err := c.store.CreateDeviceMessage(&model.DeviceMessage{
			MsgID: req.ID, FromDeviceID: fromID, ToDeviceID: toID,
			TenantID: tenantID, Type: req.Type, Payload: string(req.Payload),
			Status: status, Ts: time.Now().UnixMilli(),
		}); err != nil {
			log.Printf("mqtt peer: save message error: %v", err)
		}
	}

	// 1) 参数校验
	if req.ID == "" || req.To == "" {
		ack.Code, ack.Msg = protocol.CodeParamError, "id/to required"
		recordPeer(req.To, model.DeviceMsgRejected)
		c.publishAck(protocol.TopicPeerAck(fromID), ack)
		return
	}

	// 2) 发送方必须存在且启用（能发 peer 说明已过认证，双保险）
	fromDev, err := c.store.GetDevice(fromID)
	if err != nil || !fromDev.Enabled {
		ack.Code, ack.Msg = protocol.CodeDeviceNotRegistered, "sender not registered"
		recordPeer(req.To, model.DeviceMsgRejected)
		c.publishAck(protocol.TopicPeerAck(fromID), ack)
		return
	}

	targets, respCode, respMsg := c.resolvePeerTargets(fromDev, req.To)
	if respCode != protocol.CodeSuccess {
		ack.Code, ack.Msg = respCode, respMsg
		recordPeer(req.To, peerStatusCode(respCode))
		c.publishAck(protocol.TopicPeerAck(fromID), ack)
		return
	}

	// 3) 逐目标校验 + 投递（单目标=点对点；广播/多播=同租户授权直通）
	inboxMsg := protocol.InboxMessage{
		ID: req.ID, From: fromID, Type: req.Type,
		Payload: req.Payload, Ts: time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(inboxMsg)

	var delivered, offline int
	var firstFailCode int
	var firstFailMsg string
	for _, toDev := range targets {
		if toDev.ID == fromID {
			continue // 广播不含自己
		}
		if !toDev.Enabled {
			offline++
			recordPeer(toDev.ID, model.DeviceMsgRecipientOffline)
			continue
		}
		// 授权：同租户直通；跨租户（仅单目标模式可能产生）查白名单
		if fromDev.TenantID != toDev.TenantID {
			if _, err := c.store.GetPeerAllow(fromID, toDev.ID); err != nil {
				if firstFailCode == 0 {
					firstFailCode, firstFailMsg = protocol.CodeForbidden, "cross-tenant peer not allowed"
				}
				recordPeer(toDev.ID, model.DeviceMsgRejected)
				continue
			}
		}
		// 在线校验：平台只做在线投递，不做离线暂存
		if toDev.Status != model.DeviceStatusOnline {
			offline++
			recordPeer(toDev.ID, model.DeviceMsgRecipientOffline)
			continue
		}
		if token := c.client.Publish(protocol.TopicInbox(toDev.ID), 1, false, data); token.Error() != nil {
			log.Printf("mqtt peer: publish inbox to %s error: %v", toDev.ID, token.Error())
			offline++
			recordPeer(toDev.ID, model.DeviceMsgRecipientOffline)
			continue
		}
		delivered++
		recordPeer(toDev.ID, model.DeviceMsgDelivered)

		if c.bus != nil && fromDev.TenantID != 0 {
			c.broadcast(fromDev.TenantID, "device_peer", map[string]interface{}{
				"from_device_id": fromID, "to_device_id": toDev.ID,
				"msg_id": req.ID, "type": req.Type, "status": model.DeviceMsgDelivered,
			})
		}
	}

	total := len(targets)
	switch {
	case delivered > 0:
		ack.Code, ack.Msg = protocol.CodeSuccess, fmt.Sprintf("delivered %d/%d", delivered, total)
	case firstFailCode != 0:
		ack.Code, ack.Msg = firstFailCode, firstFailMsg
	default:
		ack.Code, ack.Msg = protocol.CodeDeviceOffline, fmt.Sprintf("no target online (0/%d)", total)
	}
	c.publishAck(protocol.TopicPeerAck(fromID), ack)
}

// resolvePeerTargets 把 to 字段解析为目标设备列表。
// 返回 (目标列表, 0, "") 或 (nil, 错误码, 错误消息)。错误码复用 peer 协议码。
func (c *Client) resolvePeerTargets(fromDev *model.Device, to string) ([]model.Device, int, string) {
	to = strings.TrimSpace(to)

	// 租户级广播
	if to == "*" {
		rows, err := c.store.ListPeerTargets(fromDev.TenantID, nil, nil, fromDev.ID, maxPeerTargets)
		if err != nil {
			log.Printf("mqtt peer: list broadcast targets error: %v", err)
			return nil, protocol.CodeParamError, "resolve targets failed"
		}
		return rows, protocol.CodeSuccess, ""
	}

	// 项目级广播 project:{id}
	if pid, ok := strings.CutPrefix(to, "project:"); ok {
		var projectID uint
		if _, err := fmt.Sscanf(strings.TrimSpace(pid), "%d", &projectID); err != nil || projectID == 0 {
			return nil, protocol.CodeParamError, "invalid project id"
		}
		proj, err := c.store.GetProjectByID(projectID)
		if err != nil {
			return nil, protocol.CodePeerNotFound, "target project not found"
		}
		if proj.TenantID != fromDev.TenantID {
			return nil, protocol.CodeForbidden, "project not in sender tenant"
		}
		rows, err := c.store.ListPeerTargets(fromDev.TenantID, &projectID, nil, fromDev.ID, maxPeerTargets)
		if err != nil {
			log.Printf("mqtt peer: list project targets error: %v", err)
			return nil, protocol.CodeParamError, "resolve targets failed"
		}
		return rows, protocol.CodeSuccess, ""
	}

	// 逗号分隔多播 / 单设备 ID
	parts := strings.Split(to, ",")
	seen := make(map[string]bool, len(parts))
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		ids = append(ids, p)
	}
	if len(ids) == 0 {
		return nil, protocol.CodeParamError, "id/to required"
	}
	if len(ids) > maxPeerTargets {
		return nil, protocol.CodeParamError, "too many targets"
	}

	if len(ids) == 1 {
		// 点对点：保持原有语义（支持跨租户白名单）
		toDev, err := c.store.GetDevice(ids[0])
		if err != nil {
			return nil, protocol.CodePeerNotFound, "target device not found"
		}
		return []model.Device{*toDev}, protocol.CodeSuccess, ""
	}

	// 多播：仅限同租户（设备 ID 字符集不含逗号，无歧义）
	rows, err := c.store.ListPeerTargets(fromDev.TenantID, nil, ids, fromDev.ID, maxPeerTargets)
	if err != nil {
		log.Printf("mqtt peer: list multicast targets error: %v", err)
		return nil, protocol.CodeParamError, "resolve targets failed"
	}
	if len(rows) == 0 {
		return nil, protocol.CodePeerNotFound, "target device not found"
	}
	return rows, protocol.CodeSuccess, ""
}

// peerStatusCode 把 peer 协议错误码映射为留痕状态。
func peerStatusCode(code int) string {
	switch code {
	case protocol.CodeForbidden:
		return model.DeviceMsgRejected
	case protocol.CodePeerNotFound:
		return model.DeviceMsgTargetNotFound
	default:
		return model.DeviceMsgRejected
	}
}

func (c *Client) publishAck(topic string, resp interface{}) {
	payload, _ := json.Marshal(resp)
	c.client.Publish(topic, 1, false, payload)
}

// handleDynRegister 一型一密动态注册：引导连接向 wendao/register/{sn}/req 请求激活。
// 三重一致校验（topic sn / body sn / body product_key）通过后做条件激活：
// 赢标者下发新签发的一机一密（code=0），并发竞争失败者/已激活回 code=4；
// 应答入队（QoS1）后踢掉引导连接，设备应保存密钥并以 username=SN 普通重连。
// 引导连接在 broker 认证阶段已做过一轮同款校验，这里按零信任原则复核（消息可能来自异常路径）。
func (c *Client) handleDynRegister(topic string, payload []byte) {
	parts := strings.Split(topic, "/")
	// wendao / register / {sn} / req
	if len(parts) != 4 {
		return
	}
	topicSN := parts[2]

	// finish 统一收口：应答入队（QoS1）→ 异步预留投递宽限 → 踢掉引导连接。
	// 所有终态（含参数错/重复激活）都走这里：并发注册时输标者与赢标者共用同一
	// username（SN&ProductKey），赢标者的踢线会断开全部引导会话，因此 code4 应答
	// 也必须先获得投递宽限，否则输标设备来不及收到"已激活"原因。
	// 踢线必须异步：本函数运行在 paho 消息派发路径上，同步 sleep 会阻塞输标请求
	// 的处理（单连接消息串行派发），导致输标应答晚于赢标踢线而永远发不出去。
	finish := func(id string, code int, msg, secret, productKey string) {
		r := protocol.RegisterResp{ID: id, Code: code, Msg: msg, DeviceSecret: secret}
		b, _ := json.Marshal(r)
		token := c.client.Publish(protocol.TopicRegisterResp(topicSN), 1, false, b)
		token.Wait()
		if err := token.Error(); err != nil {
			log.Printf("mqtt dynreg: publish reply to %s failed: %v", topicSN, err)
		}
		go func() {
			time.Sleep(dynRegKickGrace)
			c.kickBootstrap(topicSN, productKey)
		}()
	}

	var req protocol.RegisterReq
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("mqtt dynreg: invalid JSON (sn=%s): %v", topicSN, err)
		finish("", protocol.CodeParamError, "bad request", "", "")
		return
	}
	if req.SN != topicSN {
		log.Printf("mqtt dynreg: sn mismatch topic=%s body=%s", topicSN, req.SN)
		finish(req.ID, protocol.CodeParamError, "sn mismatch", "", req.ProductKey)
		return
	}
	if !protocol.ProductKeyPattern.MatchString(req.ProductKey) {
		finish(req.ID, protocol.CodeParamError, "bad product_key", "", req.ProductKey)
		return
	}
	product, err := c.store.GetProductByKey(req.ProductKey)
	if err != nil || !product.DynRegEnabled {
		finish(req.ID, protocol.CodeForbidden, "product not found or registration disabled", "", req.ProductKey)
		return
	}
	dev, err := c.store.GetDevice(req.SN)
	if err != nil || dev.ProductID != product.ID || !dev.Enabled {
		finish(req.ID, protocol.CodeDeviceNotRegistered, "sn not preregistered to product", "", req.ProductKey)
		return
	}

	secret, err := cryptopkg.RandomPassword(20)
	if err != nil {
		log.Printf("mqtt dynreg: gen secret failed sn=%s: %v", req.SN, err)
		finish(req.ID, protocol.CodeParamError, "internal error", "", req.ProductKey)
		return
	}
	won, _, err := c.store.ActivateDevice(req.SN, product.ID, cryptopkg.MustHashPassword(secret))
	if err != nil {
		log.Printf("mqtt dynreg: activate sn=%s failed: %v", req.SN, err)
		finish(req.ID, protocol.CodeParamError, "internal error", "", req.ProductKey)
		return
	}
	if !won {
		// 并发激活输标，或设备已持有一机一密
		log.Printf("mqtt dynreg: sn=%s already activated", req.SN)
		finish(req.ID, protocol.CodeAlreadyActivated, "device already activated", "", req.ProductKey)
		return
	}

	log.Printf("mqtt dynreg: activated sn=%s product=%s", req.SN, req.ProductKey)
	// 通知前端：预录设备已完成动态注册（设备卡片由“待激活”翻转）
	if c.bus != nil && dev.TenantID != 0 {
		c.bus.BroadcastToTenant(dev.TenantID, events.Message{
			Type: "device_activated",
			Data: map[string]interface{}{
				"device_id":    req.SN,
				"product_id":   product.ID,
				"activated_at": time.Now().UnixMilli(),
			},
		})
	}
	finish(req.ID, protocol.CodeSuccess, "ok", secret, req.ProductKey)
}

// kickBootstrap 踢掉某 SN 的引导连接（username="SN&ProductKey"）。
// 互踢实现未注入时静默跳过（设备可用新密钥重连触发互踢兑底）。
func (c *Client) kickBootstrap(sn, productKey string) {
	if c.kicker == nil {
		return
	}
	bootstrapUsername := sn + protocol.BootstrapSep + productKey
	kicked, err := c.kicker.KickByUsername(bootstrapUsername, "")
	if err != nil {
		log.Printf("mqtt dynreg: kick bootstrap %s failed: %v", bootstrapUsername, err)
		return
	}
	if len(kicked) > 0 {
		log.Printf("mqtt dynreg: kicked bootstrap session(s) %s: %v", bootstrapUsername, kicked)
	}
}

func (c *Client) PublishControl(deviceID string, cmd *protocol.DownlinkRequest) error {
	payload, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	token := c.client.Publish(protocol.TopicControl(deviceID), 1, false, payload)
	token.Wait()
	return token.Error()
}

// PublishPing 向 wendao/{id}/ping 发布一条探活消息（ping 在线判定模式，QoS1）。
// 不写控制日志；设备应答 wendao/{id}/ping/ack 后由 handlePingAck 刷新在线状态。
func (c *Client) PublishPing(deviceID, pingID string) error {
	payload, err := json.Marshal(protocol.DownlinkRequest{ID: pingID, Ts: time.Now().UnixMilli()})
	if err != nil {
		return err
	}
	token := c.client.Publish(protocol.TopicPing(deviceID), 1, false, payload)
	token.Wait()
	return token.Error()
}

// handlePingAck 设备探活应答：wendao/{id}/ping/ack（DownlinkResponse{id,code}）。
// 应答即证明设备可达——刷新在线状态与 last_active（与上报/连接事件同一入口）；
// 非法 JSON 忽略（不回应答，防止刷流量）。设备未注册/已禁用直接忽略。
func (c *Client) handlePingAck(deviceID string, payload []byte) {
	var ack protocol.DownlinkResponse
	if err := json.Unmarshal(payload, &ack); err != nil {
		log.Printf("mqtt ping ack: invalid JSON from %s: %v (payload: %.200s)", deviceID, err, payload)
		return
	}
	dev, err := c.store.GetDevice(deviceID)
	if err != nil || !dev.Enabled {
		return
	}
	if err := c.store.SetDeviceRuntimeStatus(deviceID, true); err != nil {
		log.Printf("mqtt ping ack: set runtime status error: %v", err)
		return
	}
	if dev.Status != model.DeviceStatusOnline {
		// 由 ping 应答恢复在线时通知前端
		if c.bus != nil && dev.TenantID != 0 {
			c.broadcast(dev.TenantID, "device_status", map[string]interface{}{
				"device_id": deviceID,
				"online":    true,
			})
		}
	}
}

func (c *Client) PublishRaw(topic string, payload []byte) error {
	token := c.client.Publish(topic, 1, false, payload)
	token.Wait()
	return token.Error()
}

func (c *Client) handleOTAProgress(deviceID string, payload []byte) {
	var prog protocol.OTAProgress
	if err := json.Unmarshal(payload, &prog); err != nil {
		log.Printf("mqtt ota progress: invalid JSON from %s: %v (payload: %.200s)", deviceID, err, payload)
		return
	}

	var tenantID uint
	if dev, err := c.store.GetDevice(deviceID); err == nil {
		tenantID = dev.TenantID
	}
	tasks, _ := c.store.ListOTATasks()
	for _, task := range tasks {
		if task.Status != model.OTATaskStatusRunning {
			continue
		}
		logs, _ := c.store.ListOTALogs(task.ID)
		for _, l := range logs {
			if l.DeviceID == deviceID && (l.Status == model.OTALogStatusPending || l.Status == model.OTALogStatusDownloading || l.Status == model.OTALogStatusInstalling) {
				c.store.UpdateOTALogStatus(deviceID, task.ID, prog.Status, prog.Progress, "")
				c.broadcast(tenantID, "ota_progress", map[string]interface{}{
					"device_id": deviceID, "task_id": task.ID, "status": prog.Status, "progress": prog.Progress,
				})
				break
			}
		}
	}
}

func (c *Client) handleOTAAck(deviceID string, payload []byte) {
	var ack protocol.OTAResponse
	if err := json.Unmarshal(payload, &ack); err != nil {
		log.Printf("mqtt ota ack: invalid JSON from %s: %v (payload: %.200s)", deviceID, err, payload)
		return
	}

	var tenantID uint
	if dev, err := c.store.GetDevice(deviceID); err == nil {
		tenantID = dev.TenantID
	}
	status := model.OTALogStatusSuccess
	errMsg := ""
	if ack.Code != 0 {
		status = model.OTALogStatusFailed
		errMsg = ack.Msg
	}

	tasks, _ := c.store.ListOTATasks()
	for _, task := range tasks {
		if task.Status != model.OTATaskStatusRunning {
			continue
		}
		logs, _ := c.store.ListOTALogs(task.ID)
		for _, l := range logs {
			if l.DeviceID == deviceID && (l.Status == model.OTALogStatusPending || l.Status == model.OTALogStatusDownloading || l.Status == model.OTALogStatusInstalling) {
				c.store.UpdateOTALogStatus(deviceID, task.ID, status, 100, errMsg)
				c.broadcast(tenantID, "ota_ack", map[string]interface{}{
					"device_id": deviceID, "task_id": task.ID, "status": status, "msg": errMsg,
				})
				break
			}
		}
		c.store.CheckAndCompleteOTATask(task.ID)
	}
}

func (c *Client) broadcast(tenantID uint, typ string, data interface{}) {
	if c.bus == nil || tenantID == 0 {
		return
	}
	c.bus.BroadcastToTenant(tenantID, events.Message{Type: typ, Data: data})
}

// Config paho 连接参数：连接独立 broker（mqtt.broker），
// 凭据为 broker.server_username/password（认证时按“平台账号”分支放行）。
type Config struct {
	Broker   string
	ClientID string
	Username string
	Password string
}
