/**
 * 设备级在线判定方式（系统默认「仅按连接」）。
 * 历史数据可能为空串，空串/未知值一律按 connection 展示与提交。
 */
export const ONLINE_MODES = [
  { value: 'connection', label: '仅按连接' },
  { value: 'report', label: '按上报时间' },
  { value: 'ping', label: '按应答信号' }
] as const

export type OnlineModeValue = (typeof ONLINE_MODES)[number]['value']

/** 空串（跟随全局的历史值）与未知值统一归一为 connection。 */
export function normalizeOnlineMode(mode?: string | null): OnlineModeValue {
  return mode === 'report' || mode === 'ping' ? mode : 'connection'
}

/** 判断是否为三种有效显式模式（空串/未知值不算）。 */
export function isExplicitMode(mode?: string | null): mode is OnlineModeValue {
  return mode === 'connection' || mode === 'report' || mode === 'ping'
}

/**
 * 解析设备实际生效的在线判定模式：设备显式值 -> 项目默认 -> 仅按连接。
 * 任一层为空串/未知值即回退到下一层（最终即「仅按连接」）。
 */
export function resolveOnlineMode(
  deviceMode?: string | null,
  projectMode?: string | null,
  systemMode: OnlineModeValue = 'connection'
): OnlineModeValue {
  if (isExplicitMode(deviceMode)) return deviceMode
  if (isExplicitMode(projectMode)) return projectMode
  return isExplicitMode(systemMode) ? systemMode : 'connection'
}

export function onlineModeLabel(mode?: string | null): string {
  return ONLINE_MODES.find(item => item.value === normalizeOnlineMode(mode))?.label ?? '仅按连接'
}

/** 将秒数格式化为中文时长：如 1天2小时30分 / 45秒。 */
export function formatTimeoutDuration(sec: number): string {
  const total = Math.max(0, Math.floor(sec))
  const d = Math.floor(total / 86400)
  const h = Math.floor((total % 86400) / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const parts: string[] = []
  if (d) parts.push(`${d}天`)
  if (h) parts.push(`${h}小时`)
  if (m) parts.push(`${m}分`)
  if (s || parts.length === 0) parts.push(`${s}秒`)
  return parts.join('')
}

/** 完整展示文案：按上报时间 · 时限5分（仅 report/ping 且设定了时限时追加）。 */
export function onlineModeText(mode?: string | null, timeoutSec = 0): string {
  const normalized = normalizeOnlineMode(mode)
  const base = onlineModeLabel(normalized)
  if ((normalized === 'report' || normalized === 'ping') && timeoutSec > 0) {
    return `${base} · 时限${formatTimeoutDuration(timeoutSec)}`
  }
  return base
}

/**
 * 完整生效文案（设备详情页用）：直接展示解析后的模式与时限。
 * 历史遗留的空串值按「仅按连接」归一，不在界面上暴露继承概念。
 */
export function effectiveOnlineModeText(
  deviceMode?: string | null,
  deviceTimeoutSec = 0,
  projectMode?: string | null,
  projectTimeoutSec = 0,
  systemTimeoutSec = 0
): string {
  const mode = resolveOnlineMode(deviceMode, projectMode)
  let timeout = 0
  if (isExplicitMode(deviceMode)) {
    timeout = deviceTimeoutSec > 0 ? deviceTimeoutSec : (isExplicitMode(projectMode) && projectTimeoutSec > 0 ? projectTimeoutSec : systemTimeoutSec)
  } else if (isExplicitMode(projectMode)) {
    timeout = projectTimeoutSec > 0 ? projectTimeoutSec : systemTimeoutSec
  }
  return onlineModeText(mode, timeout)
}
