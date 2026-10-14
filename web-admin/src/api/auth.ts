import http from './index'

export interface LoginParams {
  username: string
  password: string
  role: string
}

export interface User {
  id: number
  username: string
  role: string
  tenant_id: number
}

export interface LoginResult {
  token: string
  user: User
}

export interface UserListItem {
  id: number
  username: string
  role: string
  tenant_id: number
  created_at: string
}

export function login(params: LoginParams): Promise<{ code: number; msg: string; data: LoginResult }> {
  return http.post('/login', params)
}

export function listUsers(): Promise<{ code: number; msg: string; data: UserListItem[] }> {
  return http.get('/users')
}

export function changePassword(old_password: string, new_password: string): Promise<{ code: number; msg: string }> {
  return http.put('/users/password', { old_password, new_password })
}

export function adminResetPassword(id: number, new_password: string): Promise<{ code: number; msg: string }> {
  return http.put(`/users/${id}/password`, { new_password })
}

export function deleteUser(id: number): Promise<{ code: number; msg: string }> {
  return http.delete(`/users/${id}`)
}
