import http from './index'

// 每账号界面偏好（服务器持久化，后端做白名单合并）
export interface UIPreferences {
  /** 设备管理：true=项目卡片二级浏览；false=进入即显示全部设备（默认） */
  device_project_view: boolean
}

export function getPreferences(): Promise<{ code: number; msg: string; data: UIPreferences }> {
  return http.get('/me/preferences')
}

export function updatePreferences(data: UIPreferences): Promise<{ code: number; msg: string; data: UIPreferences }> {
  return http.put('/me/preferences', data)
}
