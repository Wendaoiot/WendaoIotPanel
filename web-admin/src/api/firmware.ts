import http from './index'
import { downloadGet } from '@/utils/download'

export interface Firmware {
  id: number
  name: string
  version: string
  url: string
  size: number
  md5: string
  description: string
  created_at: string
}

export interface OTATask {
  id: number
  firmware_id: number
  target_type: string
  target_id: string
  status: string
  created_at: string
}

export interface OTALog {
  id: number
  task_id: number
  device_id: string
  status: string
  progress: number
  error_msg: string
  created_at: string
  updated_at: string
}

export interface ControlLog {
  id: number
  device_id: string
  msg_id: string
  tags: Record<string, unknown>
  ack_code: number | null
  ack_msg: string
  status?: 'pending' | 'delivered' | 'success' | 'failed' | 'timeout' | string
  created_at: string
}

export interface ControlLogQuery {
  device_id?: string
  start?: number
  end?: number
  status?: string
  limit?: number
  offset?: number
}

export function getFirmwares(): Promise<{ code: number; msg: string; data: Firmware[] }> {
  return http.get('/firmwares')
}

export function createFirmware(data: {
  name: string
  version: string
  url: string
  size?: number
  md5?: string
  description?: string
}): Promise<{ code: number; msg: string; data: Firmware }> {
  return http.post('/firmwares', data)
}

export function deleteFirmware(id: number): Promise<{ code: number; msg: string }> {
  return http.delete('/firmwares', { data: { id } })
}

export function createOTATask(data: {
  firmware_id: number
  target_type: string
  target_id: string
}): Promise<{ code: number; msg: string; data: { task_id: number } }> {
  return http.post('/ota/tasks', data)
}

export function getOTATasks(): Promise<{ code: number; msg: string; data: OTATask[] }> {
  return http.get('/ota/tasks')
}

export function getOTALogs(taskId: number): Promise<{ code: number; msg: string; data: OTALog[] }> {
  return http.get('/ota/logs', { params: { task_id: taskId } })
}

// 控制日志删除（仅超管）：ids=按行删除；device_id/start/end=按筛选清空；all=true 可与筛选组合，无筛选=清空全部
export function deleteControlLogs(body: {
  ids?: number[]
  device_id?: string
  start?: number
  end?: number
  all?: boolean
}): Promise<{ code: number; msg: string; data: { deleted: number } }> {
  return http.delete('/control-logs', { data: body })
}

// OTA 升级日志删除（仅超管）：ids=按行删除；task_id=清空该任务日志；all=true=清空全部
export function deleteOTALogs(body: { ids?: number[]; task_id?: number; all?: boolean }): Promise<{ code: number; msg: string; data: { deleted: number } }> {
  return http.delete('/ota/logs', { data: body })
}

export function getControlLogs(query: ControlLogQuery = {}): Promise<{ code: number; msg: string; data: { list: ControlLog[]; total: number } }> {
  const params: Record<string, unknown> = {
    limit: query.limit ?? 50,
    offset: query.offset ?? 0
  }
  if (query.device_id) params.device_id = query.device_id
  if (query.start !== undefined) params.start = query.start
  if (query.end !== undefined) params.end = query.end
  if (query.status) params.status = query.status
  return http.get('/control-logs', { params })
}

// 导出控制日志 CSV（携带 JWT，Blob 下载）；status 过滤在前端完成（后端按时间/设备导出全量）
export function exportControlLogsCsv(query: ControlLogQuery = {}): Promise<void> {
  const params: Record<string, unknown> = { export: 'csv' }
  if (query.device_id) params.device_id = query.device_id
  if (query.start !== undefined) params.start = query.start
  if (query.end !== undefined) params.end = query.end
  if (query.limit !== undefined) params.limit = query.limit
  return downloadGet('/control-logs', params, 'control_logs.csv')
}
