// Package broker 装配 mochi-mqtt（MIT 协议），作为独立进程运行的 MQTT 接入服务。
//
// 职责：
//   - 装配 MQTT TCP/TLS listener（1883 / 8883）；
//   - 注册 WendaoHook 完成设备认证、ACL、设备上下线捕获；
//   - 维护 sessionIndex（username→会话）实现同设备重复连接互踢，并暴露内部踢线 HTTP；
//   - 可选 pebble 持久化（离线 QoS1 消息在 clean-session=false 设备重连后补收）。
//
// broker 与 Go 后端是两个独立部署的进程：认证直接只读 MySQL（设备/产品密钥），
// 上下线与互踢经 internal/syncapi 的内网 HTTP 通道交互，服务端业务收发以普通
// MQTT 客户端身份连入（broker.server_username/password，认证时按“平台账号”分支放行）。
package broker

import (
	"crypto/tls"
	"fmt"
	"log"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/storage/pebble"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"wendaoiotpanel/internal/model"
)

// Config 为 broker 装配参数（来源 config.Broker，见 config.Load 的默认值与环境变量）。
type Config struct {
	ListenPort      int    // MQTT TCP 端口（1883；0=禁用）
	TLSPort         int    // MQTT over TLS 端口（0=禁用）
	TLSCert         string // TLS 证书路径（TLSPort>0 时必填）
	TLSKey          string // TLS 私钥路径（TLSPort>0 时必填）
	PersistencePath string // pebble 持久化目录（空=禁用持久化）
	ServerUsername  string // 平台自身 MQTT 账号（后端业务客户端使用）
	ServerPassword  string // 平台自身 MQTT 密码
}

// AuthSource 设备/产品认证所需的只读存储接口（*store.Store 天然满足；
// 抽象为接口以便单测注入假实现）。broker 只做认证读，不写业务运行态。
type AuthSource interface {
	GetDeviceAuthRecord(deviceID string) (*model.Device, error)
	GetProductByKey(productKey string) (*model.Product, error)
	GetDevice(deviceID string) (*model.Device, error)
}

// StatusSink 接收设备上下线事件。独立进程由内部 HTTP 客户端实现
// （后端落库并广播前端）；单测可注入假实现。
type StatusSink interface {
	SetDeviceRuntime(deviceID string, online bool, clientID string, tenantID uint)
}

// Broker 独立 MQTT 接入服务。
type Broker struct {
	srv      *mqtt.Server
	cfg      Config
	store    AuthSource
	sink     StatusSink
	sessions *sessionIndex
}

// New 创建并完成 hook 注册（不监听端口，调用 Start 开始服务）。
func New(cfg Config, s AuthSource, sink StatusSink) (*Broker, error) {
	b := &Broker{
		cfg:      cfg,
		store:    s,
		sink:     sink,
		sessions: newSessionIndex(),
	}
	b.srv = mqtt.New(&mqtt.Options{
		// inline client：允许 server.Publish 从进程内直接向任意主题投递消息
		// （互踢 kicked 通知依赖它）。
		InlineClient: true,
	})

	// 认证 / ACL / 上下线 hook（必须最先注册）
	if err := b.srv.AddHook(&WendaoHook{broker: b}, nil); err != nil {
		return nil, fmt.Errorf("add wendao hook: %w", err)
	}

	// 可选持久化：会话/订阅/inflight/retained 落 pebble，进程重启后 clean-session=false
	// 的设备可补收离线期间的 QoS1 消息。
	if cfg.PersistencePath != "" {
		if err := b.srv.AddHook(new(pebble.Hook), &pebble.Options{
			Path: cfg.PersistencePath,
			Mode: pebble.NoSync,
		}); err != nil {
			return nil, fmt.Errorf("add pebble storage hook: %w", err)
		}
	}
	return b, nil
}

// Server 暴露底层 mochi server，供内部踢线 HTTP 处理器调用 KickByUsername。
func (b *Broker) Server() *mqtt.Server { return b.srv }

// Start 装配 listener 并开始接受连接（非阻塞，Serve 在后台 goroutine 运行）。
func (b *Broker) Start() error {
	if b.cfg.ListenPort > 0 {
		if err := b.srv.AddListener(listeners.NewTCP(listeners.Config{
			ID:      fmt.Sprintf("tcp%d", b.cfg.ListenPort),
			Address: fmt.Sprintf(":%d", b.cfg.ListenPort),
		})); err != nil {
			return fmt.Errorf("add tcp listener: %w", err)
		}
	}

	if b.cfg.TLSPort > 0 {
		cert, err := tls.LoadX509KeyPair(b.cfg.TLSCert, b.cfg.TLSKey)
		if err != nil {
			return fmt.Errorf("load tls cert: %w", err)
		}
		if err := b.srv.AddListener(listeners.NewTCP(listeners.Config{
			ID:        fmt.Sprintf("tls%d", b.cfg.TLSPort),
			Address:   fmt.Sprintf(":%d", b.cfg.TLSPort),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		})); err != nil {
			return fmt.Errorf("add tls listener: %w", err)
		}
	}

	go func() {
		if err := b.srv.Serve(); err != nil {
			log.Printf("broker serve exited: %v", err)
		}
	}()
	log.Printf("broker started: tcp=%d tls=%d persistence=%q server_user=%s",
		b.cfg.ListenPort, b.cfg.TLSPort, b.cfg.PersistencePath, b.cfg.ServerUsername)
	return nil
}

// Close 停止 broker 并断开全部客户端（进程退出时调用）。
func (b *Broker) Close() error {
	log.Println("broker closing")
	return b.srv.Close()
}

// KickByUsername 踢掉某 username（设备ID / 引导用户名）名下的全部会话，
// excludeClientID 用于互踢时排除新连接自身；传空踢全部。
// 返回成功断开的 clientID 列表。
func (b *Broker) KickByUsername(username, excludeClientID string) ([]string, error) {
	if username == "" {
		return nil, nil
	}
	var kicked []string
	for _, cl := range b.sessions.List(username) {
		if cl.ID == excludeClientID {
			continue
		}
		// MQTT5 向客户端发 DISCONNECT(0x98 administrative action) 后断开；v3 直接断开。
		if err := b.srv.DisconnectClient(cl, packets.ErrAdministrativeAction); err != nil {
			log.Printf("broker kick %s (client %s) error: %v", username, cl.ID, err)
			continue
		}
		kicked = append(kicked, cl.ID)
	}
	return kicked, nil
}
