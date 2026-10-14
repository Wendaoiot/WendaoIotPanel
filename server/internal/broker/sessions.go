package broker

import (
	"strings"
	"sync"

	mqtt "github.com/mochi-mqtt/server/v2"
)

// sessionIndex 维护 username → clientID → *mqtt.Client 的会话索引，
// 供互踢按 username 精确断开（broker 进程内会话索引，无需外部管理接口）。
//
// 生命周期：OnSessionEstablished 登记；OnDisconnect 注销（含互踢导致的断开）。
// 同一 username 允许多个 clientID 并存（引导连接与普通连接、异常双连等），
// 互踢语义由调用方决定排除哪个 clientID。
type sessionIndex struct {
	mu         sync.RWMutex
	byUsername map[string]map[string]*mqtt.Client
}

func newSessionIndex() *sessionIndex {
	return &sessionIndex{byUsername: make(map[string]map[string]*mqtt.Client)}
}

// Add 登记一个会话。
func (s *sessionIndex) Add(username, clientID string, cl *mqtt.Client) {
	if username == "" || clientID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUsername[username] == nil {
		s.byUsername[username] = make(map[string]*mqtt.Client)
	}
	s.byUsername[username][clientID] = cl
}

// Remove 注销一个会话。
func (s *sessionIndex) Remove(username, clientID string) {
	if username == "" || clientID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.byUsername[username]; m != nil {
		delete(m, clientID)
		if len(m) == 0 {
			delete(s.byUsername, username)
		}
	}
}

// List 返回某 username 当前登记的全部客户端（快照；供踢线遍历）。
func (s *sessionIndex) List(username string) []*mqtt.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*mqtt.Client
	for _, cl := range s.byUsername[username] {
		out = append(out, cl)
	}
	return out
}

// hookClientUsername 从 mochi Client 取 username（CONNECT 阶段由 broker 填充）。
func hookClientUsername(cl *mqtt.Client) string {
	if cl == nil {
		return ""
	}
	return string(cl.Properties.Username)
}

// isBootstrapUsername 判断是否一型一密引导用户名（"SN&ProductKey"）。
func isBootstrapUsername(username string) bool {
	return strings.Contains(username, bootstrapSep)
}
