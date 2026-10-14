import http from './index'
import { downloadGet } from '@/utils/download'

export interface Device {
  id: string
  project_id: number
  name: string
  status: number
  enabled?: boolean
  /** 设备级在线判定：connection=仅按连接(系统默认) / report=按上报时间 / ping=按应答信号；历史空串等同 connection */
  online_mode?: string
  /** 判定离线时限（秒），0=沿用全局时限；connection 下不使用，上限 604800 */
  offline_timeout_sec?: number
  /** 一型一密：所属产品 ID；0/缺省=传统一机一密设备 */
  product_id?: number
  /** 一型一密：动态注册激活时间；null=已预录待激活 */
  activated_at?: string | null
  first_ts: number
  last_active: string | null
  created_at: string
}

/** 单条/批量预录的逐行结果（成功行无 msg 字段） */
export interface PreregisterRow {
  id: string
  ok: boolean
  msg?: string
}

export interface PreregisterResult {
  total: number
  succeeded: number
  failed: number
  results: PreregisterRow[]
}

export interface DeviceTag {
  id: number
  device_id: string
  tag_key: string
  name?: string
  unit?: string
  interface: string
  formula: string
}

export interface DeviceDataPoint {
  id: number
  device_id: string
  msg_id: string
  ts: number
  device_ts?: number
  data: Record<string, unknown>
  version?: string
  created_at: string
}

export interface ControlSendResult {
  msg_id: string
  status?: string
  hint?: string
}

export interface ControlResult {
  msg_id: string
}

export interface ControlStatus {
  msg_id: string
  status: 'pending' | 'delivered' | 'success' | 'failed' | 'timeout' | string
  ack_code: number | null
  ack_msg: string
}

// 项目自定义控制命令
export interface ControlCommand {
  id: number
  project_id: number
  device_id?: string // 空字符串 = 项目通用
  name: string
  tag_key: string
  value: number
  icon?: string
  sort?: number
  danger?: boolean
}

export interface DeviceDataQuery {
  start?: number
  end?: number
  limit?: number
  offset?: number
}

// 数据删除条件（三选一）：按行 ID / 时间范围（毫秒，可只给一端）/ 全部
export interface DeviceDataDeleteQuery {
  ids?: number[]
  start?: number
  end?: number
  all?: boolean
}

export function getDevices(projectId?: number): Promise<{ code: number; msg: string; data: Device[] }> {
  const params: Record<string, number> = {}
  if (projectId !== undefined && projectId !== null) {
    params.project_id = projectId
  }
  return http.get('/devices', { params })
}

// 设备状态筛选（与后端 /devices/page 对齐）：''=全部
export type DeviceStatusFilter = '' | 'online' | 'offline' | 'pending' | 'disabled'

export interface DevicePageQuery {
  project_id?: number
  keyword?: string
  status?: DeviceStatusFilter
  page?: number
  page_size?: number
}

export interface DevicePage {
  total: number
  page: number
  page_size: number
  items: Device[]
}

// 分页检索设备（千台规模）：服务端关键字(SN/名称)、状态过滤、在线优先+最近活跃排序。
export function getDevicesPage(query: DevicePageQuery): Promise<{ code: number; msg: string; data: DevicePage }> {
  const params: Record<string, string | number> = {}
  if (query.project_id !== undefined && query.project_id !== null) params.project_id = query.project_id
  if (query.keyword && query.keyword.trim()) params.keyword = query.keyword.trim()
  if (query.status) params.status = query.status
  params.page = query.page && query.page > 0 ? query.page : 1
  params.page_size = query.page_size && query.page_size > 0 ? query.page_size : 24
  return http.get('/devices/page', { params })
}

export function getDevice(deviceId: string): Promise<{ code: number; msg: string; data: Device }> {
  return http.get(`/devices/${deviceId}`)
}

export function createDevice(data: { id: string; project_id: number; name: string }): Promise<{ code: number; msg: string; data: { device: Device; device_secret: string; secret_note: string } }> {
  return http.post('/devices', data)
}

// 一型一密：预录一台待激活设备（后端批量接口传单条）。
// 返回逐行结果：ok=true 待首次 mqtts 引导连接动态注册激活；ok=false 时 msg 为失败原因。
export function preregisterDevice(data: {
  product_key: string
  project_id: number
  sn: string
  name?: string
}): Promise<{ code: number; msg: string; data: PreregisterResult }> {
  return http.post('/devices/batch-preregister', {
    product_key: data.product_key,
    project_id: data.project_id,
    items: [{ id: data.sn, name: data.name || '' }]
  })
}

// 一型一密：清空一机一密，设备回到待激活（产品凭证重新引导注册）。
export function reactivateDevice(deviceId: string): Promise<{ code: number; msg: string; data: { id: string; activated: boolean } }> {
  return http.post(`/devices/${deviceId}/reactivate`, {})
}

export function updateDevice(
  deviceId: string,
  data: { name?: string; project_id?: number; status?: number; online_mode?: string; offline_timeout_sec?: number }
): Promise<{ code: number; msg: string; data: Device }> {
  return http.put(`/devices/${deviceId}`, data)
}

export function deleteDevice(deviceId: string): Promise<{ code: number; msg: string }> {
  return http.delete(`/devices/${deviceId}`)
}

// 启用/禁用设备
export function setDeviceEnabled(deviceId: string, enabled: boolean): Promise<{ code: number; msg: string }> {
  return http.put(`/devices/${deviceId}/enabled`, { enabled })
}

// 重置设备密钥（明文仅本次返回）
export function resetDeviceSecret(deviceId: string): Promise<{ code: number; msg: string; data: { device_id: string; device_secret: string; secret_note: string } }> {
  return http.post(`/devices/${deviceId}/secret/reset`)
}

export function addDeviceTag(deviceId: string, data: { tag_key: string; name?: string; unit?: string; interface: string; formula: string }): Promise<{ code: number; msg: string; data: DeviceTag }> {
  return http.post(`/devices/${deviceId}/tags`, data)
}

export function getDeviceTags(deviceId: string): Promise<{ code: number; msg: string; data: DeviceTag[] }> {
  return http.get(`/devices/${deviceId}/tags`)
}

export function removeDeviceTag(deviceId: string, id: number): Promise<{ code: number; msg: string }> {
  return http.delete(`/devices/${deviceId}/tags`, { data: { id } })
}

export function getDeviceData(
  deviceId: string,
  query: DeviceDataQuery = {}
): Promise<{ code: number; msg: string; data: { list: DeviceDataPoint[]; total: number; limit: number; offset: number } }> {
  const params: Record<string, unknown> = {
    limit: query.limit ?? 100,
    offset: query.offset ?? 0
  }
  if (query.start !== undefined) params.start = query.start
  if (query.end !== undefined) params.end = query.end
  return http.get(`/devices/${deviceId}/data`, { params })
}

export function sendDeviceControl(deviceId: string, tags: Record<string, number>): Promise<{ code: number; msg: string; data: ControlSendResult }> {
  return http.post(`/devices/${deviceId}/control`, { tags })
}

// 删除设备数据（仅超管；后端按 ids / start+end / all 三选一处理）
export function deleteDeviceData(deviceId: string, data: DeviceDataDeleteQuery): Promise<{ code: number; msg: string; data: { deleted: number } }> {
  return http.delete(`/devices/${deviceId}/data`, { data })
}

// 查询单条控制指令状态（pending/delivered/success/failed/timeout）
export function getControlStatus(deviceId: string, msgId: string): Promise<{ code: number; msg: string; data: ControlStatus }> {
  return http.get(`/devices/${deviceId}/control/${msgId}`)
}

// ===== 项目自定义控制命令 =====

export function listCommands(projectId: number, deviceId?: string): Promise<{ code: number; msg: string; data: ControlCommand[] }> {
  const params: Record<string, unknown> = {}
  if (deviceId) params.device_id = deviceId
  return http.get(`/projects/${projectId}/commands`, { params })
}

export function createCommand(data: { project_id: number; device_id?: string; name: string; tag_key: string; value: number; icon?: string; sort?: number; danger?: boolean }): Promise<{ code: number; msg: string; data: ControlCommand }> {
  return http.post(`/projects/${data.project_id}/commands`, data)
}

export function updateCommand(id: number, data: Partial<Omit<ControlCommand, 'id'>>): Promise<{ code: number; msg: string; data: ControlCommand }> {
  return http.put(`/commands/${id}`, data)
}

export function deleteCommand(id: number): Promise<{ code: number; msg: string }> {
  return http.delete(`/commands/${id}`)
}

// ===== CSV 导出（携带 JWT，Blob 下载）=====

export function exportDeviceDataCsv(deviceId: string, query: DeviceDataQuery = {}): Promise<void> {
  const params: Record<string, unknown> = { export: 'csv' }
  if (query.start !== undefined) params.start = query.start
  if (query.end !== undefined) params.end = query.end
  if (query.limit !== undefined) params.limit = query.limit
  if (query.offset !== undefined) params.offset = query.offset
  return downloadGet(`/devices/${deviceId}/data`, params, `device_data_${deviceId}.csv`)
}
