import { getToken } from '@/utils/auth'

// H5 走 Vite 代理，小程序直连服务器
// #ifdef H5
const BASE_URL = '/api/v1'
// #endif
// #ifdef MP-WEIXIN
// 小程序端必须直连服务器（无代理）：
// 1. 在 .env / manifest 环境配置中设置 VITE_API_BASE_URL，且必须是 HTTPS 地址（如 https://api.example.com）；
// 2. 登录微信公众平台「开发管理 - 开发设置 - 服务器域名」，把该域名加入 request 合法域名。
// 未配置时 BASE_URL 退化为相对路径，小程序请求必然失败，这里给出明确报错便于排查。
const MP_SERVER_BASE = String(import.meta.env.VITE_API_BASE_URL || '').replace(/\/+$/, '')
if (!MP_SERVER_BASE) {
  console.error(
    '[web-app] 小程序端未配置 VITE_API_BASE_URL，接口不可用。请配置 HTTPS 服务器地址，并在微信小程序后台添加 request 合法域名。'
  )
}
const BASE_URL = `${MP_SERVER_BASE}/api/v1`
// #endif

interface RequestOptions {
  url: string
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  data?: Record<string, any>
  header?: Record<string, string>
  showLoading?: boolean
}

interface Response<T = any> {
  code: number
  data: T
  msg: string
}

function buildUrl(url: string, params?: Record<string, any>): string {
  if (!params) return url
  const qs = Object.entries(params)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
    .join('&')
  return qs ? `${url}?${qs}` : url
}

function request<T = any>(options: RequestOptions): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...options.header
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  if (options.showLoading) {
    uni.showLoading({ title: '加载中...', mask: true })
  }

  const method = options.method || 'GET'
  const fullUrl = method === 'GET' ? buildUrl(`${BASE_URL}${options.url}`, options.data) : `${BASE_URL}${options.url}`

  return new Promise<T>((resolve, reject) => {
    uni.request({
      url: fullUrl,
      method,
      data: method !== 'GET' ? options.data : undefined,
      header: headers,
      timeout: 15000,
      success: (res) => {
        if (options.showLoading) {
          uni.hideLoading()
        }
        const body = res.data as Response<T>
        if (res.statusCode === 200 && body.code === 0) {
          resolve(body.data)
        } else if (res.statusCode === 401) {
          uni.redirectTo({ url: '/pages/login/index' })
          reject(new Error(body.msg || '未授权，请重新登录'))
        } else {
          uni.showToast({ title: body.msg || '请求失败', icon: 'none' })
          reject(new Error(body.msg || `请求错误: ${res.statusCode}`))
        }
      },
      fail: (err) => {
        if (options.showLoading) {
          uni.hideLoading()
        }
        uni.showToast({ title: '网络异常，请检查连接', icon: 'none' })
        reject(err)
      }
    })
  })
}

export function get<T = any>(url: string, data?: Record<string, any>): Promise<T> {
  return request<T>({ url, method: 'GET', data })
}

export function post<T = any>(url: string, data?: Record<string, any>): Promise<T> {
  return request<T>({ url, method: 'POST', data })
}

export function put<T = any>(url: string, data?: Record<string, any>): Promise<T> {
  return request<T>({ url, method: 'PUT', data })
}

export function del<T = any>(url: string, data?: Record<string, any>): Promise<T> {
  return request<T>({ url, method: 'DELETE', data })
}

export interface LoginParams {
  username: string
  password: string
  role: string
}

export interface LoginResult {
  token: string
  user: {
    id: number
    username: string
    role: string
    tenant_id: number
  }
}

export interface Project {
  id: number
  name: string
  description?: string
  created_at?: string
}

export interface DeviceTag {
  id: number
  device_id: string
  tag_key: string
  interface: string
  formula: string
}

export interface Device {
  id: string
  project_id: number
  name: string
  status: number
  created_at: string
}

export interface ProjectTag {
  id: number
  project_id: number
  tag_key: string
  tag_name: string
  unit: string
  data_type: string
  writable: boolean
}

/** 控制下发的标签值：标签 key -> 下发值（数值或字符串） */
export type ControlTagValue = string | number

export interface ControlPayload {
  tags: Record<string, ControlTagValue>
}

/** 控制下发结果：POST /devices/:deviceId/control */
export interface ControlResult {
  msg_id: string
  status: string
  hint?: string
}

/** 控制回执状态（与设备 ACK 阶段对齐） */
export type ControlAckStatus = 'pending' | 'delivered' | 'success' | 'failed' | 'timeout'

/** 控制执行结果查询：GET /devices/:deviceId/control/:msgId */
export interface ControlStatus {
  msg_id: string
  status: ControlAckStatus
  ack_code?: number
  ack_msg?: string
}

export const authApi = {
  login: (params: LoginParams) => post<LoginResult>('/login', params)
}

export const projectApi = {
  list: () => get<Project[]>('/projects'),
  getTags: (projectId: number | string) => get<ProjectTag[]>(`/projects/${projectId}/tags`),
  getData: (projectId: number | string) => get<ProjectData>(`/projects/${projectId}/data`)
}

export interface DeviceValue {
  device_id: string
  device_name: string
  value: number
  ts: number
  status: number
}

export interface TagData {
  tag_key: string
  tag_name: string
  unit: string
  data_type: string
  writable: boolean
  devices: DeviceValue[]
}

export interface ProjectData {
  project_id: number
  tags: TagData[]
}

export interface DeviceDataItem {
  id: number
  device_id: string
  data: Record<string, any>
  ts: number
}

export interface DeviceDataListResult {
  list: DeviceDataItem[]
  total: number
}

export interface DeviceDataQuery {
  limit?: number
  offset?: number
}

export const deviceApi = {
  list: (projectId: number | string) => get<Device[]>('/devices', { project_id: Number(projectId) }),
  getTags: (deviceId: number | string) => get<DeviceTag[]>(`/devices/${deviceId}/tags`),
  getData: (deviceId: number | string, params?: DeviceDataQuery) =>
    get<DeviceDataListResult>(`/devices/${deviceId}/data`, params as Record<string, unknown>),
  getLatestData: (deviceId: number | string) => get<{ list: Record<string, any>[] }>(`/devices/${deviceId}/data`, { limit: 1 }),
  control: (deviceId: number | string, payload: ControlPayload) =>
    post<ControlResult>(`/devices/${deviceId}/control`, payload),
  getControlStatus: (deviceId: number | string, msgId: string) =>
    get<ControlStatus>(`/devices/${deviceId}/control/${encodeURIComponent(msgId)}`)
}

export const userApi = {
  changePassword: (old_password: string, new_password: string) =>
    put('/users/password', { old_password, new_password })
}
