// 设备间通信（D2D）类型与 API

import http from './index'

export interface DevicePeerMessage {
  id: number
  msg_id: string
  from_device_id: string
  to_device_id: string
  tenant_id: number
  type: string
  payload: string // JSON 字符串
  status: 'delivered' | 'recipient_offline' | 'rejected' | 'target_not_found' | string
  ts: number
  created_at: string
}

export interface DevicePeerAllow {
  id: number
  from_device_id: string
  to_device_id: string
  tenant_id: number
  remark: string
  created_at: string
}

export function listPeerMessages(
  deviceId: string,
  query: { limit?: number; offset?: number } = {}
): Promise<{
  code: number
  msg: string
  data: { list: DevicePeerMessage[]; total: number; limit: number; offset: number }
}> {
  return http.get(`/devices/${deviceId}/peer/messages`, {
    params: { limit: query.limit ?? 50, offset: query.offset ?? 0 }
  })
}

export function listPeerAllows(
  query: { limit?: number; offset?: number } = {}
): Promise<{
  code: number
  msg: string
  data: { list: DevicePeerAllow[]; total: number; limit: number; offset: number }
}> {
  return http.get('/peer/allows', { params: { limit: query.limit ?? 100, offset: query.offset ?? 0 } })
}

export function createPeerAllow(data: {
  from_device_id: string
  to_device_id: string
  remark?: string
}): Promise<{ code: number; msg: string; data: DevicePeerAllow }> {
  return http.post('/peer/allows', data)
}

export function deletePeerAllow(id: number): Promise<{ code: number; msg: string }> {
  return http.delete(`/peer/allows/${id}`)
}
