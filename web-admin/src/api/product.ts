import http from './index'

export interface Product {
  id: number
  product_key: string
  tenant_id: number
  name: string
  /** 动态注册开关：关闭后设备引导连接将被拒绝 */
  dyn_reg_enabled: boolean
  created_at: string
  updated_at: string
}

export interface ProductSecretResult {
  product?: Product
  product_key?: string
  product_secret: string
  secret_note: string
}

export function getProducts(tenantId?: number): Promise<{ code: number; msg: string; data: Product[] }> {
  const params = tenantId ? { tenant_id: tenantId } : {}
  return http.get('/products', { params })
}

export function createProduct(data: { name: string; tenant_id?: number }): Promise<{
  code: number
  msg: string
  data: { product: Product; product_secret: string; secret_note: string }
}> {
  return http.post('/products', data)
}

export function updateProduct(
  productKey: string,
  data: { name?: string; dyn_reg_enabled?: boolean }
): Promise<{ code: number; msg: string }> {
  return http.put(`/products/${productKey}`, data)
}

export function resetProductSecret(productKey: string): Promise<{ code: number; msg: string; data: ProductSecretResult }> {
  return http.post(`/products/${productKey}/secret/reset`)
}

export function deleteProduct(productKey: string): Promise<{ code: number; msg: string }> {
  return http.delete(`/products/${productKey}`)
}
