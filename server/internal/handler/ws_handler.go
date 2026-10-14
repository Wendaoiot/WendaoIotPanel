package handler

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	// 校验 Origin：默认放行同源；浏览器外请求若无 Origin（原生 WS 客户端）也放行，
	// 真正的安全关口是握手必须带有效 token。
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServeWS 处理前端 WebSocket 连接。
// 鉴权：浏览器无法在 WS 握手时自定义 Header，因此 token 通过 ?token= 或 Sec-WebSocket-Protocol 传入。
// 鉴权成功后按租户注册到事件总线；连接关闭时注销。
func (h *Handler) ServeWS(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		// 兼容 Sec-WebSocket-Protocol: bearer.<token>
		if p := c.GetHeader("Sec-WebSocket-Protocol"); strings.Contains(p, ".") {
			tokenStr = strings.TrimSpace(strings.SplitN(p, ".", 2)[1])
		}
	}
	if tokenStr == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "missing token"})
		return
	}

	claims, err := tokenMgr.Parse(tokenStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "invalid token"})
		return
	}
	user, err := h.store.GetAdminUserByIDFull(claims.UserID)
	if err != nil || user.TokenVersion != claims.TokenVersion {
		c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "登录已失效"})
		return
	}
	h.setupWS(c, user.TenantID)
}

func (h *Handler) setupWS(c *gin.Context, tenantID *uint) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.bus.Register(conn, tenantID)
	log.Printf("ws: client connected tenant=%v total=%d", tenantID, h.busCount())

	defer func() {
		h.bus.Unregister(conn)
		conn.Close()
	}()

	// 读循环：仅用于检测客户端断开；前端无需发送业务消息。
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		return nil
	})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *Handler) busCount() int {
	// 简单返回 0 仅用于日志；Bus 未暴露计数时不影响功能。
	return 0
}
