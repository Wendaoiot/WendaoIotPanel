import http from './index'

export interface Tenant {
  id: number
  name: string
  admin_user?: string
  admin_pwd?: string
  created_at: string
}

export function getTenants(): Promise<{ code: number; msg: string; data: Tenant[] }> {
  return http.get('/tenants')
}

export function createTenant(
  name: string,
  admin_pwd?: string,
  admin_username?: string
): Promise<{ code: number; msg: string; data: Tenant }> {
  const data: Record<string, string> = { name }
  if (admin_pwd) data.admin_pwd = admin_pwd
  if (admin_username) data.admin_username = admin_username
  return http.post('/tenants', data)
}

export function updateTenant(id: number, name: string): Promise<{ code: number; msg: string; data: Tenant }> {
  return http.put(`/tenants/${id}`, { name })
}

export function deleteTenant(id: number): Promise<{ code: number; msg: string }> {
  return http.delete(`/tenants/${id}`)
}
