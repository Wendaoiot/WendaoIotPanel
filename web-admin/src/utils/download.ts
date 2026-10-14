import http from '@/api/index'

/** 触发浏览器下载一个 Blob */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // 延迟释放，确保下载已开始
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/**
 * 携带 JWT 发起 GET 下载请求（window.open 无法带 Authorization 头）。
 * 后端在导出失败时会返回 JSON 错误体（Content-Type: application/json），
 * 这里识别并抛出带后端 msg 的错误；成功则直接触发下载。
 */
export async function downloadGet(
  url: string,
  params: Record<string, unknown>,
  filename: string
): Promise<void> {
  const resp = await http.get(url, { params, responseType: 'blob' })
  const blob = resp.data as Blob
  if (blob && blob.type && blob.type.includes('application/json')) {
    const text = await blob.text()
    let msg = '导出失败'
    try {
      const j = JSON.parse(text)
      if (j && j.msg) msg = j.msg
    } catch {
      // 非 JSON 文本，使用默认提示
    }
    throw new Error(msg)
  }
  downloadBlob(blob, filename)
}
