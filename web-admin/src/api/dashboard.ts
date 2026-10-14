import http from './index'

export interface DashboardStats {
  total_tenants: number
  total_projects: number
  total_devices: number
  online_devices: number
  disabled_devices: number
  pending_devices: number
  // 消息量（数据库计数）
  messages_in_24h: number
  messages_out_24h: number
  messages_in_total_db: number
  messages_out_total_db: number
  // 实时速率（当前服务节点内存计数，详见后端 metrics 包）
  in_rate: number
  out_rate: number
  in_total: number
  out_total: number
  series_in: number[]
  series_out: number[]
}

// 历史流量分桶窗口（与后端 store.TrafficRanges 对齐）
export type TrafficRangeKey = '1h' | '24h' | '7d'

export interface TrafficSeries {
  range: TrafficRangeKey
  bucket_sec: number
  start_sec: number
  end_sec: number
  points: number
  series_in: number[]
  series_out: number[]
}

export function getDashboardStats(): Promise<{ code: number; msg: string; data: DashboardStats }> {
  return http.get('/dashboard/stats')
}

export function getDashboardTraffic(
  range: TrafficRangeKey
): Promise<{ code: number; msg: string; data: TrafficSeries }> {
  return http.get('/dashboard/traffic', { params: { range } })
}
