const TOKEN_KEY = 'iot_auth_token'
const USER_KEY = 'iot_auth_user'

export function getToken(): string {
  return uni.getStorageSync(TOKEN_KEY) || ''
}

export function setToken(token: string): void {
  uni.setStorageSync(TOKEN_KEY, token)
}

export function removeToken(): void {
  uni.removeStorageSync(TOKEN_KEY)
}

export function getUser(): Record<string, any> | null {
  const raw = uni.getStorageSync(USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

export function setUser(user: Record<string, any>): void {
  uni.setStorageSync(USER_KEY, JSON.stringify(user))
}

export function removeUser(): void {
  uni.removeStorageSync(USER_KEY)
}

export function isLoggedIn(): boolean {
  return !!getToken()
}

export function getUserRole(): string {
  const user = getUser()
  return user?.role || ''
}

export function getTenantId(): number | null {
  const user = getUser()
  return user?.tenant_id ?? null
}

export function logout(): void {
  removeToken()
  removeUser()
}
