import http from './index'

export interface Project {
  id: number
  tenant_id: number
  name: string
  // 项目级在线判定默认（''/0 = 沿用系统默认；设备未显式覆盖时生效）
  online_mode: string
  offline_timeout_sec: number
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
  created_at: string
}

export function getProjects(tenantId?: number): Promise<{ code: number; msg: string; data: Project[] }> {
  const params = tenantId ? { tenant_id: tenantId } : {}
  return http.get('/projects', { params })
}

export function createProject(data: { tenant_id: number; name: string }): Promise<{ code: number; msg: string; data: Project }> {
  return http.post('/projects', data)
}

export function updateProject(id: number, data: { name?: string; tenant_id?: number }): Promise<{ code: number; msg: string; data: Project }> {
  return http.put(`/projects/${id}`, data)
}

export function deleteProject(id: number): Promise<{ code: number; msg: string }> {
  return http.delete(`/projects/${id}`)
}

// 项目文件夹设备计数角标：{ [project_id]: {total, online, offline, pending, disabled} }
export interface ProjectDeviceStats {
  total: number
  online: number
  offline: number
  pending: number
  disabled: number
}

export function getProjectDeviceStats(tenantId?: number): Promise<{ code: number; msg: string; data: Record<string, ProjectDeviceStats> }> {
  const params = tenantId ? { tenant_id: tenantId } : {}
  return http.get('/projects/device-stats', { params })
}

// 项目级在线判定默认；online_mode 传 '' 表示沿用系统默认，offline_timeout_sec=0 沿用系统时限
export function updateProjectSettings(
  id: number,
  data: { online_mode: string; offline_timeout_sec: number }
): Promise<{ code: number; msg: string }> {
  return http.put(`/projects/${id}/settings`, data)
}

// 一键应用：把项目默认显式写入该项目全部设备（覆盖设备各自设置），返回受影响设备数
export function applyProjectOnlineDefault(
  id: number,
  data: { online_mode: string; offline_timeout_sec: number }
): Promise<{ code: number; msg: string; data: { affected: number } }> {
  return http.post(`/projects/${id}/apply-online-default`, data)
}

export function addProjectTag(projectId: number, data: { tag_key: string; tag_name: string; unit: string; data_type: string; writable: boolean }): Promise<{ code: number; msg: string; data: ProjectTag }> {
  return http.post(`/projects/${projectId}/tags`, data)
}

export function getProjectTags(projectId: number): Promise<{ code: number; msg: string; data: ProjectTag[] }> {
  return http.get(`/projects/${projectId}/tags`)
}

export function removeProjectTag(projectId: number, id: number): Promise<{ code: number; msg: string }> {
  return http.delete(`/projects/${projectId}/tags`, { data: { id } })
}
