<template>
  <div class="page-container dashboard" v-loading="loading">
    <!-- 资源概览：只放“总量”类指标，状态明细在下方设备面板、消息指标在消息面板，不重复 -->
    <header class="overview">
      <div v-if="isSuperAdmin" class="ov-item">
        <el-icon class="ov-ic ov-blue"><OfficeBuilding /></el-icon>
        <div><div class="ov-num">{{ stats.total_tenants }}</div><div class="ov-label">租户</div></div>
      </div>
      <div v-if="isSuperAdmin" class="ov-divider" />
      <div class="ov-item">
        <el-icon class="ov-ic ov-cyan"><FolderOpened /></el-icon>
        <div><div class="ov-num">{{ stats.total_projects }}</div><div class="ov-label">项目</div></div>
      </div>
      <div class="ov-divider" />
      <div class="ov-item">
        <el-icon class="ov-ic ov-slate"><Cpu /></el-icon>
        <div><div class="ov-num">{{ stats.total_devices }}</div><div class="ov-label">设备总数</div></div>
      </div>
      <div class="ov-divider" />
      <div class="ov-item">
        <el-icon class="ov-ic ov-green"><Connection /></el-icon>
        <div><div class="ov-num ov-text-green">{{ stats.online_devices }}</div><div class="ov-label">在线 · {{ onlinePercent }}%</div></div>
      </div>

      <!-- 设备状态分布：环图 + 四态明细，收进概览条右侧同一容器 -->
      <div class="ov-sep-status"></div>
      <div class="ov-status">
        <svg viewBox="0 0 120 120" class="donut ov-donut">
          <circle cx="60" cy="60" r="46" class="donut-track" />
          <circle
            v-for="seg in donutSegs"
            :key="seg.key"
            cx="60" cy="60" r="46"
            :stroke="seg.color"
            :stroke-dasharray="seg.dash"
            :stroke-dashoffset="seg.offset"
            class="donut-seg"
          />
          <text x="60" y="55" text-anchor="middle" class="donut-total">{{ stats.total_devices }}</text>
          <text x="60" y="73" text-anchor="middle" class="donut-label">设备总数</text>
        </svg>
        <ul class="donut-legend ov-legend">
          <li v-for="seg in donutSegs" :key="seg.key">
            <i :style="{ background: seg.color }"></i>
            <span class="dl-name">{{ seg.name }}</span>
            <b>{{ seg.value }}</b>
          </li>
        </ul>
      </div>
    </header>

    <!-- 消息监控独占整行：时间档（实时/1h/24h/7d）× 指标视图（速率/累计），同一序列前端派生 -->
    <main class="panels">
      <section class="panel panel-message">
        <header class="panel-title">
          <span class="pt-name"><el-icon class="pt-ic"><DataLine /></el-icon>消息监控</span>
          <div class="legend">
            <span class="legend-item"><i class="dot dot-in"></i>流入</span>
            <span class="legend-item"><i class="dot dot-out"></i>流出</span>
          </div>
        </header>

        <div class="msg-toolbar">
          <el-radio-group v-model="timeRange" size="small" @change="onRangeChange">
            <el-radio-button value="live">实时</el-radio-button>
            <el-radio-button value="1h">近1小时</el-radio-button>
            <el-radio-button value="24h">近24小时</el-radio-button>
            <el-radio-button value="7d">近7天</el-radio-button>
          </el-radio-group>
          <el-radio-group v-model="metricView" size="small">
            <el-radio-button value="total">累计</el-radio-button>
            <el-radio-button value="rate">速率</el-radio-button>
          </el-radio-group>
        </div>

        <div class="msg-current">
          <div class="mc-item mc-box-in">
            <span class="mc-label"><i class="dot dot-in"></i>消息流入</span>
            <div><b class="mc-num mc-in">{{ currentIn }}</b><em>{{ valUnit }}</em></div>
          </div>
          <div class="mc-item mc-box-out">
            <span class="mc-label"><i class="dot dot-out"></i>消息流出</span>
            <div><b class="mc-num mc-out">{{ currentOut }}</b><em>{{ valUnit }}</em></div>
          </div>
        </div>

        <div class="chart-wrap">
          <svg class="rate-svg" viewBox="0 0 800 230" preserveAspectRatio="xMidYMid meet">
            <defs>
              <linearGradient id="grad-in" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="var(--wd-success)" stop-opacity="0.22" />
                <stop offset="100%" stop-color="var(--wd-success)" stop-opacity="0" />
              </linearGradient>
              <linearGradient id="grad-out" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="var(--wd-warning)" stop-opacity="0.18" />
                <stop offset="100%" stop-color="var(--wd-warning)" stop-opacity="0" />
              </linearGradient>
            </defs>
            <line v-for="g in 4" :key="'g'+g" :x1="padL" :x2="chartRight" :y1="gridY(g)" :y2="gridY(g)" class="grid" />
            <line :x1="padL" :x2="chartRight" :y1="chartBottom" :y2="chartBottom" class="axis" />
            <text v-for="g in 5" :key="'y'+g" :x="padL-8" :y="gridY(g-1)+4" text-anchor="end" class="svg-label">{{ yLabels[g-1] }}</text>

            <!-- 首载/切档：最终图表（真实数据）自左向右揭示 0.9s，揭示出的每一刻都是终态形状；
                 30s 静默刷新/实时轮询数据直接替换，不重播 -->
            <g :key="drawKey" class="series" :class="{ 'is-drawing': drawing }">
              <path v-if="inArea" :d="inArea" fill="url(#grad-in)" class="area-in" />
              <path v-if="outArea" :d="outArea" fill="url(#grad-out)" class="area-out" />
              <path v-if="inPath" :d="inPath" class="line-in" />
              <path v-if="outPath" :d="outPath" class="line-out" />
              <circle v-if="inLast" :cx="inLast[0]" :cy="inLast[1]" r="3.5" class="line-end line-end-in" />
              <circle v-if="outLast" :cx="outLast[0]" :cy="outLast[1]" r="3.5" class="line-end line-end-out" />
            </g>
            <text v-for="(t, i) in xTicks" :key="'x'+i" :x="t.x" :y="chartBottom+20" :text-anchor="t.anchor" class="svg-label svg-x">{{ t.label }}</text>
            <text v-if="seriesEmpty() && !chartLoading" :x="400" :y="(topPad+chartBottom)/2" text-anchor="middle" class="svg-empty">{{ emptyText }}</text>
          </svg>
        </div>
        <p class="chart-hint">{{ chartHint }}</p>

        <footer class="msg-sum">
          <span v-if="timeRange === 'live'">近 24h：流入 <b class="ms-in">{{ formatNum(stats.messages_in_24h) }}</b> · 流出 <b class="ms-out">{{ formatNum(stats.messages_out_24h) }}</b></span>
          <span v-else>本区间合计：流入 <b class="ms-in">{{ rangeSumIn }}</b> · 流出 <b class="ms-out">{{ rangeSumOut }}</b></span>
          <span class="ms-total">数据库累计：流入 {{ formatNum(stats.messages_in_total_db) }} · 流出 {{ formatNum(stats.messages_out_total_db) }}</span>
        </footer>
      </section>
    </main>

    <footer class="sys-bar">
      <el-icon><CircleCheck /></el-icon> 服务运行中 · v1.0.0
      <span class="sys-sep">|</span>{{ authStore.user?.username || '-' }}
      （{{ authStore.user?.role === 'super_admin' ? '超级管理员' : '租户管理员' }}）
    </footer>
  </div>
</template>

<script setup lang="ts">
import { reactive, computed, onMounted, onUnmounted, ref } from 'vue'
import {
  OfficeBuilding, FolderOpened, Cpu, Connection,
  DataLine, CircleCheck
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import {
  getDashboardStats, getDashboardTraffic,
  type DashboardStats, type TrafficRangeKey
} from '@/api/dashboard'
import { useRealtime } from '@/composables/useRealtime'

const authStore = useAuthStore()
const loading = ref(false)
const refreshSec = 2

const stats = reactive<DashboardStats>({
  total_tenants: 0,
  total_projects: 0,
  total_devices: 0,
  online_devices: 0,
  disabled_devices: 0,
  pending_devices: 0,
  messages_in_24h: 0,
  messages_out_24h: 0,
  messages_in_total_db: 0,
  messages_out_total_db: 0,
  in_rate: 0,
  out_rate: 0,
  in_total: 0,
  out_total: 0,
  series_in: [],
  series_out: []
})

const isSuperAdmin = computed(() => authStore.role === 'super_admin')

const onlinePercent = computed(() => {
  if (stats.total_devices === 0) return 0
  return Math.round(stats.online_devices / stats.total_devices * 100)
})

// 离线 = 总数 - 在线 - 禁用 - 待激活（待激活设备本身未上线，单独成类；与 donut 口径一致）
const offlineCount = computed(() =>
  Math.max(0, stats.total_devices - stats.online_devices - stats.disabled_devices - stats.pending_devices)
)

function formatNum(n: number): string {
  return (n || 0).toLocaleString('zh-CN')
}

// 大数字与折线均由同一序列派生：累计=前缀和，速率=桶计数折算到分钟/小时。
// ===== 消息监控：时间档 × 指标视图 =====
type TimeRangeKey = 'live' | TrafficRangeKey
type MetricView = 'rate' | 'total'
// 默认近 24 小时（该档通常有数据，且累计视图有持续爬升的图线效果）
const timeRange = ref<TimeRangeKey>('24h')
// 默认展示累计（有持续爬升的图线效果）；速率为次视图
const metricView = ref<MetricView>('total')
// 仅首载/切换时间档时为 true（显示绘制动画）；30s 后台刷新与实时 2s 轮询静默，不闪动画
const chartLoading = ref(true)
const hist = ref<{ bucketSec: number; startSec: number; in: number[]; out: number[] } | null>(null)

const RANGE_BUCKET: Record<TimeRangeKey, number> = { live: 1, '1h': 60, '24h': 600, '7d': 3600 }
// 速率单位时长：实时/近1小时按“条/分钟”，近24小时/近7天按“条/小时”
const RATE_UNIT_SEC: Record<TimeRangeKey, number> = { live: 60, '1h': 60, '24h': 3600, '7d': 3600 }
// 真实曲线描画动画：非静默加载完成后递增 drawKey 使序列 <g> 重挂载，
// CSS 动画把最终折线从左到右描出（0.9s），描完即终态。
const drawing = ref(false)
const drawKey = ref(0)
let drawTimer: ReturnType<typeof setTimeout> | null = null
const DRAW_ANIM_MS = 1000

function startDraw() {
  if (drawTimer) clearTimeout(drawTimer)
  drawKey.value++
  drawing.value = true
  drawTimer = setTimeout(() => { drawing.value = false }, DRAW_ANIM_MS)
}
const HIST_REFRESH_SEC = 30

// 原始每桶“条数”序列：实时=内存环 30 点(1s)；历史=接口分桶
const rawIn = computed<number[]>(() =>
  timeRange.value === 'live' ? stats.series_in : hist.value?.in ?? [])
const rawOut = computed<number[]>(() =>
  timeRange.value === 'live' ? stats.series_out : hist.value?.out ?? [])

// 展示序列：速率=每桶条数折算到速率单位（分钟/小时）；累计=前缀和
function perRate(a: number[], bucketSec: number, unitSec: number): number[] {
  return a.map(v => v * unitSec / bucketSec)
}
function cumulative(a: number[]): number[] {
  let acc = 0
  return a.map(v => (acc += v))
}

const viewIn = computed<number[]>(() => {
  const b = RANGE_BUCKET[timeRange.value]
  return metricView.value === 'rate' ? perRate(rawIn.value, b, RATE_UNIT_SEC[timeRange.value]) : cumulative(rawIn.value)
})
const viewOut = computed<number[]>(() => {
  const b = RANGE_BUCKET[timeRange.value]
  return metricView.value === 'rate' ? perRate(rawOut.value, b, RATE_UNIT_SEC[timeRange.value]) : cumulative(rawOut.value)
})

const valUnit = computed(() => {
  if (metricView.value === 'total') return '条'
  return RATE_UNIT_SEC[timeRange.value] >= 3600 ? '条/小时' : '条/分钟'
})
// 速率视图显示“本区间平均速率”（总量÷区间时长），避免低流量下最后一个桶恒为 0；
// 累计视图仍显示前缀和末端（=区间累计）
const avgRateIn = computed(() => {
  const a = rawIn.value
  if (!a.length) return 0
  const totalSec = a.length * RANGE_BUCKET[timeRange.value]
  return a.reduce((x, y) => x + y, 0) / totalSec * RATE_UNIT_SEC[timeRange.value]
})
const avgRateOut = computed(() => {
  const a = rawOut.value
  if (!a.length) return 0
  const totalSec = a.length * RANGE_BUCKET[timeRange.value]
  return a.reduce((x, y) => x + y, 0) / totalSec * RATE_UNIT_SEC[timeRange.value]
})
const currentIn = computed(() =>
  chartLoading.value ? '—' : formatChartValue(metricView.value === 'rate' ? avgRateIn.value : viewIn.value[viewIn.value.length - 1] ?? 0))
const currentOut = computed(() =>
  chartLoading.value ? '—' : formatChartValue(metricView.value === 'rate' ? avgRateOut.value : viewOut.value[viewOut.value.length - 1] ?? 0))

const rangeSumIn = computed(() => formatNum(rawIn.value.reduce((a, b) => a + b, 0)))
const rangeSumOut = computed(() => formatNum(rawOut.value.reduce((a, b) => a + b, 0)))

const emptyText = computed(() =>
  timeRange.value === 'live' ? '暂无实时消息' : '该时间段暂无消息')
const chartHint = computed(() => {
  const unitSuffix = metricView.value === 'rate' ? ` · 速率单位：${valUnit.value}` : ''
  if (timeRange.value === 'live') {
    return `最近 30 秒（${refreshSec}s 刷新）· 当前服务节点近似值${unitSuffix}`
  }
  const b = RANGE_BUCKET[timeRange.value]
  const bucketLabel = b >= 3600 ? `${b / 3600} 小时` : b >= 60 ? `${b / 60} 分钟` : `${b} 秒`
  return `每 ${bucketLabel}一个采样点 · ${HIST_REFRESH_SEC}s 自动刷新 · 数据来自数据库消息记录${unitSuffix}`
})

// ===== 折线（viewBox 800x230，绘图区 x:[padL,chartRight] y:[20,180]） =====
const padL = 46
const chartRight = 790
// 数据点相对网格左右各内缩数个单位，首末点不贴轴端
const plotLeft = padL + 8
const plotRight = chartRight - 8
const topPad = 20
const chartBottom = 180

function gridY(g: number): number {
  return chartBottom - g * ((chartBottom - topPad) / 4)
}

// Y 轴动态范围：按当前序列最大值取整，再预留 1/5 顶部空白
// （轴顶 ≥ 最大值 ×1.25，曲线最高点不超过绘图区 4/5）
const PLOT_HEADROOM = 1.25
const yAxisMax = computed(() => {
  // 速率可能远小于 1（如 0.1 条/分钟），下限不能钳到 1，否则小数速率被压成贴轴平线
  const max = Math.max(1e-9, ...viewIn.value, ...viewOut.value)
  return niceStep(max * PLOT_HEADROOM) * 4
})
const yLabels = computed(() => {
  const max = yAxisMax.value / 4
  return [0, 1, 2, 3, 4].map(g => formatAxis(max * g))
})

function niceStep(max: number): number {
  // 入参已乘 1.25（顶部 1/5 留白）；这里取“每格步长”，必须向上取整，
  // 否则留白会被取整吞掉、曲线顶到绘图区上缘（紧贴上方流入/流出数值卡）。
  // 梯级集合 {1,2,3,5,10}（乘以 10 的幂）：整数时得 0/3/6/9/12，小数速率时得 0/0.05/0.1/0.15。
  const raw = max / 4
  const pow = Math.pow(10, Math.floor(Math.log10(raw)))
  const n = raw / pow
  const m = n <= 1 ? 1 : n <= 2 ? 2 : n <= 3 ? 3 : n <= 5 ? 5 : 10
  return m * pow
}

// 统一两位小数精度：<100 保留最多两位（去尾零）；≥100 取整，轴面更整洁
function fmt2(v: number): string {
  if (v >= 100 || Number.isInteger(v)) return formatNum(Math.round(v))
  return (Math.round(v * 100) / 100).toLocaleString('zh-CN')
}
function formatAxis(v: number): string {
  if (v >= 10000) return `${Math.round(v / 1000) / 10}w`
  return fmt2(v)
}
function formatChartValue(v: number): string {
  return fmt2(v)
}

function buildPath(series: number[]): { line: string; area: string; points: Array<readonly [number, number]> } {
  if (!series.length || series.every(v => v === 0)) return { line: '', area: '', points: [] }
  // 不能钳到 1：小数速率（0.1 条/分）会被压成贴轴平线
  const maxV = Math.max(1e-9, yAxisMax.value)
  const n = series.length
  const left = plotLeft
  const h = chartBottom - topPad
  const w = plotRight - left
  const points = series.map((v, i) => {
    const x = left + (n === 1 ? w / 2 : (i / (n - 1)) * w)
    const y = chartBottom - Math.min(v, maxV) / maxV * h
    return [x, y] as const
  })
  const line = points.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ')
  const area = `${line} L${points[n - 1][0].toFixed(1)},${chartBottom} L${points[0][0].toFixed(1)},${chartBottom} Z`
  return { line, area, points }
}

const inGeom = computed(() => buildPath(viewIn.value))
const outGeom = computed(() => buildPath(viewOut.value))
const inPath = computed(() => inGeom.value.line)
const outPath = computed(() => outGeom.value.line)
const inArea = computed(() => inGeom.value.area)
const outArea = computed(() => outGeom.value.area)
// 序列末端当前值圆点（白色描边）
const inLast = computed(() => {
  const p = inGeom.value.points
  return p.length ? p[p.length - 1] : null
})
const outLast = computed(() => {
  const p = outGeom.value.points
  return p.length ? p[p.length - 1] : null
})

function seriesEmpty(): boolean {
  return viewIn.value.every(v => v === 0) && viewOut.value.every(v => v === 0)
}

// X 轴时间刻度（约 6 个）
function pad2(n: number): string { return String(n).padStart(2, '0') }
const xTicks = computed(() => {
  const n = timeRange.value === 'live'
    ? stats.series_in.length
    : (hist.value?.in.length ?? 0)
  if (!n) return []
  const bucketSec = RANGE_BUCKET[timeRange.value]
  const nowSec = Math.floor(Date.now() / 1000)
  const startSec = timeRange.value === 'live'
    ? nowSec - (n - 1)
    : hist.value?.startSec ?? nowSec - (n - 1) * bucketSec
  const left = plotLeft, w = plotRight - plotLeft
  const nowD = new Date()
  const tickCount = Math.min(6, n)
  const ticks: { x: number; label: string; anchor: string }[] = []
  for (let t = 0; t < tickCount; t++) {
    const i = tickCount === 1 ? 0 : Math.round(t * (n - 1) / (tickCount - 1))
    const d = new Date((startSec + i * bucketSec) * 1000)
    let label: string
    const hm = `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
    // 跨天的分钟档刻度前缀日期；小时间档(7d)显示月-日；秒档显示到秒
    if (bucketSec >= 3600) label = `${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
    else if (bucketSec === 1) label = `${hm}:${pad2(d.getSeconds())}`
    else if (d.getFullYear() !== nowD.getFullYear() || d.getMonth() !== nowD.getMonth() || d.getDate() !== nowD.getDate()) {
      label = `${pad2(d.getMonth() + 1)}/${pad2(d.getDate())} ${hm}`
    } else label = hm
    const anchor = t === 0 ? 'start' : t === tickCount - 1 ? 'end' : 'middle'
    ticks.push({ x: left + (n === 1 ? w / 2 : (i / (n - 1)) * w), label, anchor })
  }
  return ticks
})

// ===== 设备状态环 =====
const donutSegs = computed(() => {
  const total = Math.max(1, stats.total_devices)
  const C = 2 * Math.PI * 46
  const defs = [
    { key: 'online', name: '在线', value: stats.online_devices, color: '#67c23a' },
    { key: 'offline', name: '离线', value: offlineCount.value, color: '#c0c4cc' },
    { key: 'pending', name: '待激活', value: stats.pending_devices, color: '#e6a23c' },
    { key: 'disabled', name: '已禁用', value: stats.disabled_devices, color: '#f56c6c' }
  ]
  let acc = 0
  return defs.map(d => {
    const frac = d.value / total
    const seg = { ...d, dash: `${frac * C} ${C}`, offset: -acc * C }
    acc += frac
    return seg
  })
})

async function fetchStats(showLoading = false) {
  if (showLoading) loading.value = true
  try {
    const res = await getDashboardStats()
    if (res.data) Object.assign(stats, res.data)
  } catch {
  } finally {
    loading.value = false
  }
}

// silent=后台定时刷新：数据静默替换，不触发绘制动画
async function fetchTraffic(range: TrafficRangeKey, silent = false) {
  if (!silent) chartLoading.value = true
  try {
    const res = await getDashboardTraffic(range)
    if (res.data) {
      hist.value = {
        bucketSec: res.data.bucket_sec,
        startSec: res.data.start_sec,
        in: res.data.series_in,
        out: res.data.series_out
      }
      // 真实数据到齐后再描画；静默刷新不重播
      if (!silent) {
        chartLoading.value = false
        // 等 DOM 先套用新 d，再用 key 重挂载触发描画，保证画的是当前数据
        requestAnimationFrame(() => startDraw())
      }
    }
  } catch {
  } finally {
    if (!silent && !hist.value) chartLoading.value = false
  }
}

function onRangeChange(v: string | number | boolean | undefined) {
  const r = String(v) as TimeRangeKey
  hist.value = null
  if (r !== 'live') {
    fetchTraffic(r as TrafficRangeKey)
  } else {
    // 实时档数据随 stats 轮询到达，无需拉取；用当前已轮询到的序列描画一次
    chartLoading.value = false
    requestAnimationFrame(() => startDraw())
  }
}

// 2s 定轮询实时速率与总量；历史档 30s 刷新；设备上下线/激活事件防抖重取
const { onMessage } = useRealtime()
let offRealtime: (() => void) | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null
let histTimer: ReturnType<typeof setInterval> | null = null
let refreshTimer: ReturnType<typeof setTimeout> | null = null

onMounted(() => {
  fetchStats(true)
  // 默认 24h：首载拉取一次，期间显示折线绘制动画
  fetchTraffic('24h')
  pollTimer = setInterval(() => fetchStats(), refreshSec * 1000)
  histTimer = setInterval(() => {
    if (timeRange.value !== 'live') fetchTraffic(timeRange.value as TrafficRangeKey, true)
  }, HIST_REFRESH_SEC * 1000)
  offRealtime = onMessage((msg) => {
    if (msg.type !== 'device_status' && msg.type !== 'device_activated') return
    if (refreshTimer) clearTimeout(refreshTimer)
    refreshTimer = setTimeout(() => fetchStats(), 1500)
  })
})

onUnmounted(() => {
  offRealtime?.()
  if (pollTimer) clearInterval(pollTimer)
  if (histTimer) clearInterval(histTimer)
  if (refreshTimer) clearTimeout(refreshTimer)
  if (drawTimer) clearTimeout(drawTimer)
})
</script>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ===== 资源概览条：一行总量，分隔线分段，无卡片盒子 ===== */
.overview {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 18px 22px;
  padding: 16px 22px;
  background: var(--wd-surface);
  border: 1px solid var(--wd-border);
  border-radius: var(--wd-radius-lg);
  box-shadow: var(--wd-shadow-sm);
}
.ov-item {
  display: flex;
  align-items: center;
  gap: 12px;
}
/* 右侧设备状态分布（与总量指标同一容器，右对齐） */
.ov-sep-status {
  width: 1px;
  height: 72px;
  margin-left: auto;
  background: var(--wd-border-lighter);
}
.ov-status {
  display: flex;
  align-items: center;
  gap: 18px;
  flex-shrink: 0;
}
.ov-donut { width: 92px; height: 92px; flex-shrink: 0; }
.ov-legend {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: none;
  min-width: 0;
  display: grid;
  grid-template-columns: auto auto;
  gap: 4px 20px;
}
.ov-legend li {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  border: none;
  font-size: 12px;
  color: var(--wd-text-regular);
  white-space: nowrap;
}
.ov-legend b { font-size: 13px; font-variant-numeric: tabular-nums; color: var(--wd-text-primary); }
.ov-ic {
  font-size: 20px;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.ov-blue { color: #409eff; background: #ecf5ff; }
.ov-cyan { color: #13c2c2; background: #e6fffb; }
.ov-slate { color: #606266; background: #f4f4f5; }
.ov-green { color: var(--wd-success); background: var(--wd-success-bg); }
.ov-num {
  font-size: 22px;
  font-weight: 700;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
  color: var(--wd-text-primary);
}
.ov-text-green { color: var(--wd-success); }
.ov-label { font-size: 12.5px; color: var(--wd-text-secondary); }
.ov-divider { width: 1px; height: 28px; background: var(--wd-border-lighter); }

/* ===== 消息监控独占整行 ===== */
.panels {
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
  align-items: stretch;
}
.panel {
  background: var(--wd-surface);
  border: 1px solid var(--wd-border);
  border-radius: var(--wd-radius-lg);
  box-shadow: var(--wd-shadow-sm);
  padding: 18px 20px;
  min-width: 0;
}
.panel-title {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 14px;
  font-weight: 600;
  color: var(--wd-text-primary);
  margin-bottom: 16px;
}
.panel-message > .panel-title { justify-content: space-between; }
.panel-title .el-icon { color: var(--wd-primary); font-size: 16px; }
.pt-name { display: inline-flex; align-items: center; gap: 7px; min-width: 0; }
.pt-ic { color: var(--wd-success); font-size: 16px; flex-shrink: 0; }

.legend { display: flex; gap: 14px; font-size: 12px; color: var(--wd-text-regular); }
.legend-item { display: inline-flex; align-items: center; gap: 5px; }
.dot { width: 9px; height: 9px; border-radius: 50%; display: inline-block; flex-shrink: 0; }
.dot-in { background: var(--wd-success); }
.dot-out { background: var(--wd-warning); }

/* ===== 设备状态环（内嵌概览条） ===== */
.donut-track { fill: none; stroke: var(--wd-border-lighter); stroke-width: 13; }
.donut-seg {
  fill: none;
  stroke-width: 13;
  transform: rotate(-90deg);
  transform-origin: 60px 60px;
  transition: stroke-dasharray .4s ease, stroke-dashoffset .4s ease;
}
.donut-total { fill: var(--wd-text-primary); font-size: 23px; font-weight: 700; }
.donut-label { fill: var(--wd-text-secondary); font-size: 10.5px; }
.ov-legend i { width: 9px; height: 9px; border-radius: 2px; flex-shrink: 0; }
.ov-legend .dl-name { flex: none; }

/* ===== 消息监控 ===== */
.msg-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}
.msg-toolbar :deep(.el-radio-button__inner) { padding: 6px 12px; }

.msg-current {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-bottom: 14px;
}
.mc-item {
  padding: 10px 14px;
  border-radius: var(--wd-radius-md);
  min-width: 0;
}
.mc-box-in { background: var(--wd-success-bg); }
.mc-box-out { background: var(--wd-warning-bg); }
.mc-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--wd-text-secondary);
  margin-bottom: 2px;
}
.mc-item div { display: flex; align-items: baseline; gap: 5px; }
.mc-num { font-size: 26px; font-weight: 700; line-height: 1.1; font-variant-numeric: tabular-nums; }
.mc-in { color: var(--wd-success); }
.mc-out { color: var(--wd-warning); }
.mc-item em { font-style: normal; font-size: 12px; color: var(--wd-text-secondary); }

/* 等比缩放（meet）：保证图中文字/网格不被拉伸变形 */
.chart-wrap { position: relative; width: 100%; }
.rate-svg { width: 100%; height: auto; display: block; }
.grid { stroke: var(--wd-border-lighter); stroke-width: 1; }
.axis { stroke: var(--wd-border); stroke-width: 1; }
.svg-label { fill: var(--wd-text-placeholder); font-size: 12px; }
.svg-x { font-size: 11px; }
.svg-empty { fill: var(--wd-text-secondary); font-size: 16px; }
.line-in, .line-out { fill: none; stroke-linecap: round; stroke-linejoin: round; }
.line-in { stroke: var(--wd-success); stroke-width: 2; }
.line-out { stroke: var(--wd-warning); stroke-width: 2; }
.line-end { stroke: #fff; stroke-width: 1.5; }
.line-end-in { fill: var(--wd-success); }
.line-end-out { fill: var(--wd-warning); }

/* 真实曲线描画：数据到齐后序列 <g> 以 is-drawing 重挂载，
   右裁剪边自 100% 收到 0，最终图表从左向右揭示；揭示出的每一帧都是终态形状，
   故前段长时间零消息时前半段本就贴基线（正常，不是空动画） */
.series.is-drawing {
  clip-path: inset(0 100% 0 0);
  animation: chart-reveal 0.9s cubic-bezier(0.23, 1, 0.32, 1) forwards;
}
@keyframes chart-reveal {
  to { clip-path: inset(0 0 0 0); }
}
@media (prefers-reduced-motion: reduce) {
  .series.is-drawing { animation: none; clip-path: none; }
}
.chart-hint { margin: 8px 0 0; font-size: 11.5px; color: var(--wd-text-placeholder); }

.msg-sum {
  display: flex;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--wd-border-lighter);
  font-size: 12.5px;
  color: var(--wd-text-secondary);
}
.msg-sum b { font-variant-numeric: tabular-nums; }
.ms-in { color: var(--wd-success); }
.ms-out { color: var(--wd-warning); }
.ms-total { color: var(--wd-text-placeholder); }

.sys-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--wd-text-placeholder);
  padding: 2px 4px;
}
.sys-bar .el-icon { color: var(--wd-success); }
.sys-sep { margin: 0 4px; }

@media (max-width: 900px) {
  .panels { grid-template-columns: 1fr; }
}
</style>
