import { ref } from 'vue'

// 全局实时 WebSocket（单例）：GET /api/v1/ws?token=<JWT>
// 服务端推送 { type, data }，type ∈ device_data / control_ack / ota_progress / ota_ack / device_status
// 全应用共享一条连接，由 Layout 登录后 connect、登出 disconnect；组件只订阅消息，自动重连（5s）。

export interface RealtimeMessage {
  type: string
  data: Record<string, unknown>
}

export type RealtimeHandler = (msg: RealtimeMessage) => void

const connected = ref(false)
const lastMessage = ref<RealtimeMessage | null>(null)

let ws: WebSocket | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let manualClosed = false
const handlers = new Set<RealtimeHandler>()

function currentToken(): string {
  return localStorage.getItem('token') || ''
}

function buildUrl(token: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}/api/v1/ws?token=${encodeURIComponent(token)}`
}

function clearReconnect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

function connect() {
  manualClosed = false
  const token = currentToken()
  if (!token) return
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
    return
  }

  ws = new WebSocket(buildUrl(token))

  ws.onopen = () => {
    connected.value = true
  }

  ws.onmessage = (ev: MessageEvent) => {
    try {
      const msg = JSON.parse(String(ev.data)) as RealtimeMessage
      if (!msg || typeof msg.type !== 'string') return
      lastMessage.value = msg
      handlers.forEach((h) => {
        try {
          h(msg)
        } catch {
          // 单个 handler 异常不影响其他订阅者
        }
      })
    } catch {
      // 忽略无法解析的消息
    }
  }

  ws.onclose = () => {
    connected.value = false
    ws = null
    // token 仍在（非主动登出）时自动重连
    if (!manualClosed && currentToken()) {
      clearReconnect()
      reconnectTimer = setTimeout(connect, 5000)
    }
  }

  ws.onerror = () => {
    // close 事件会紧随其后触发，统一在 onclose 处理重连
    ws?.close()
  }
}

function disconnect() {
  manualClosed = true
  clearReconnect()
  if (ws) {
    ws.close()
    ws = null
  }
  connected.value = false
}

/** 订阅推送消息，返回取消订阅函数（组件卸载时调用，不影响全局连接） */
function onMessage(handler: RealtimeHandler): () => void {
  handlers.add(handler)
  // 首次订阅时若连接尚未建立（例如直接深链进入内页），兜底拉起
  if (!ws && currentToken()) connect()
  return () => handlers.delete(handler)
}

export function useRealtime() {
  return { connected, lastMessage, connect, disconnect, onMessage }
}
