// 统一时间格式化工具。
// 约定：后端所有时间戳字段（ts / device_ts / first_ts 等）均为 Unix 毫秒，
// 渲染一律走这里，禁止再做 *1000 换算。
// 兼容 last_active 等 GORM 时间字段返回的 ISO 8601 字符串。

function parseDate(input: number | string | null | undefined): Date | null {
  if (input === null || input === undefined || input === '') return null
  // 纯数字字符串/数字：按毫秒时间戳处理
  const d = typeof input === 'string' && /^\d+$/.test(input.trim())
    ? new Date(Number(input))
    : new Date(input)
  return Number.isNaN(d.getTime()) ? null : d
}

/** 完整日期时间，如 2026/9/3 14:30:05 */
export function formatTs(ms: number | string | null | undefined): string {
  const d = parseDate(ms)
  if (!d) return '-'
  return d.toLocaleString('zh-CN', { hour12: false })
}

/** 仅时分秒，如 14:30:05（图表坐标轴等场景） */
export function formatTime(ms: number | string | null | undefined): string {
  const d = parseDate(ms)
  if (!d) return '-'
  return d.toLocaleTimeString('zh-CN', { hour12: false })
}

/** 仅日期，如 2026/9/3 */
export function formatDate(ms: number | string | null | undefined): string {
  const d = parseDate(ms)
  if (!d) return '-'
  return d.toLocaleDateString('zh-CN')
}
