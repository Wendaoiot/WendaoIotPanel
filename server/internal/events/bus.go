// Package events 提供服务端向 WebSocket 前端实时推送的事件总线。
// MQTT 上行数据/ack/OTA 进度通过 Bus 投递，按租户隔离；超级管理员可接收全平台事件。
package events

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message 是推送给前端的实时事件信封。
type Message struct {
	Type string      `json:"type"` // device_data | control_ack | ota_progress | ota_ack | device_status
	Data interface{} `json:"data"`
}

type client struct {
	conn     *websocket.Conn
	tenantID *uint // nil 表示超级管理员（接收全部）
}

// Bus 维护所有已鉴权的 WebSocket 连接，按租户广播。
type Bus struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*client
}

func NewBus() *Bus {
	return &Bus{clients: make(map[*websocket.Conn]*client)}
}

// Register 注册一个已鉴权连接。tenantID 为 nil 表示超管。
func (b *Bus) Register(conn *websocket.Conn, tenantID *uint) {
	b.mu.Lock()
	b.clients[conn] = &client{conn: conn, tenantID: tenantID}
	b.mu.Unlock()
}

func (b *Bus) Unregister(conn *websocket.Conn) {
	b.mu.Lock()
	delete(b.clients, conn)
	b.mu.Unlock()
}

// wsWriteTimeout 是单次 WS 写操作的上限。
// 客户端不读数据时 TCP 缓冲终会填满，无死线的 WriteMessage 会永久阻塞调用方
// （paho order=true 时即整个 MQTT 路由 goroutine），因此写前必须设死线。
const wsWriteTimeout = 5 * time.Second

// BroadcastToTenant 向指定租户（以及所有超管连接）推送事件。
// 任一客户端写失败（超时/对端断开）都会被移除，绝不阻塞广播循环。
func (b *Bus) BroadcastToTenant(tenantID uint, msg Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	b.mu.RLock()
	var targets []*client
	for _, cl := range b.clients {
		if cl.tenantID == nil || *cl.tenantID == tenantID {
			targets = append(targets, cl)
		}
	}
	b.mu.RUnlock()

	for _, cl := range targets {
		_ = cl.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
		if err := cl.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Printf("ws: broadcast write failed, dropping client: %v", err)
			// 慢/死客户端：注销并关闭，让其读循环退出触发重连。
			b.Unregister(cl.conn)
			go cl.conn.Close()
		}
	}
}
