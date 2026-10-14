// Package syncapi 定义 MQTT broker 进程与 Go 后端进程之间的内部 HTTP 同步契约。
//
// 两个进程独立部署（与 MySQL 同为基础服务），仅在 compose 内网通信，不对宿主暴露：
//
//   - broker → 后端：POST /internal/v1/device-status 上报设备连接/断开（含租户），
//     由后端写运行态并经 WebSocket 广播前端；
//   - 后端 → broker：POST /internal/v1/kick  按 username 踢除会话（互踢/禁用/重置激活）。
//
// 所有请求带共享密钥头 X-Sync-Key，两端必须配置一致（WQ_SYNC_SECRET）。
package syncapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HeaderName 携带内部共享密钥的请求头。
const HeaderName = "X-Sync-Key"

const (
	// PathDeviceStatus broker 上报设备上下线。
	PathDeviceStatus = "/internal/v1/device-status"
	// PathKick 后端请求 broker 踢除会话。
	PathKick = "/internal/v1/kick"
)

// DeviceStatusRequest 由 broker 在设备会话建立/断开时上报。
type DeviceStatusRequest struct {
	DeviceID string `json:"device_id"`
	Online   bool   `json:"online"`
	ClientID string `json:"client_id"`
	TenantID uint   `json:"tenant_id"` // 0 表示无法解析租户，后端仍更新运行态但不广播
}

// KickRequest 由后端在互踢/禁用/重置激活时发往 broker。
type KickRequest struct {
	// Username 为 MQTT 用户名：普通设备即设备 ID，引导连接为 "SN&ProductKey"。
	Username string `json:"username"`
	// ExcludeClientID 排除本次新连接自身；空表示踢掉该 username 下全部会话。
	ExcludeClientID string `json:"exclude_client_id"`
}

// KickResponse 返回被断开的 clientID 列表。
type KickResponse struct {
	Kicked []string `json:"kicked"`
}

// Client 是内部 HTTP 调用方（带共享密钥与超时）。
type Client struct {
	BaseURL string       // 对端基础地址，如 http://wendao:8080 或 http://broker:18090
	Secret  string       // X-Sync-Key 共享密钥
	HTTP    *http.Client // 零值时使用默认客户端
}

// NewClient 构造内部调用客户端，baseURL 末尾的斜杠会被去除。
func NewClient(baseURL, secret string) *Client {
	return &Client{
		BaseURL: trimSlash(baseURL),
		Secret:  secret,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) post(ctx context.Context, path string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(HeaderName, c.Secret)

	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	httpResp, err := hc.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(httpResp.Body, 512))
		return fmt.Errorf("internal api %s -> %s: %s", path, httpResp.Status, strings.TrimSpace(string(snippet)))
	}
	if resp != nil {
		return json.NewDecoder(httpResp.Body).Decode(resp)
	}
	return nil
}

// ReportDeviceStatus 上报设备上下线。
func (c *Client) ReportDeviceStatus(ctx context.Context, req DeviceStatusRequest) error {
	return c.post(ctx, PathDeviceStatus, req, nil)
}

// Kick 请求 broker 踢除指定 username 的会话。
func (c *Client) Kick(ctx context.Context, req KickRequest) (*KickResponse, error) {
	out := &KickResponse{}
	if err := c.post(ctx, PathKick, req, out); err != nil {
		return nil, err
	}
	return out, nil
}

func trimSlash(s string) string {
	return strings.TrimRight(s, "/")
}
