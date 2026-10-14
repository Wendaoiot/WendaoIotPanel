<template>
  <div class="tab-pane">
    <div class="tab-toolbar">
        <el-select
          v-model="selectedTag"
          placeholder="选择数据项"
          style="width: 150px;"
        >
          <el-option
            v-for="tag in availableTags"
            :key="tag"
            :label="getTagName(tag)"
            :value="tag"
          />
        </el-select>
        <el-select
          v-model="timeRange"
          placeholder="时间范围"
          style="width: 120px; margin-right: 12px;"
        >
          <el-option label="最近1小时" value="1h" />
          <el-option label="最近6小时" value="6h" />
          <el-option label="最近24小时" value="24h" />
        </el-select>
        <el-switch
          v-model="autoRefresh"
          active-text="自动刷新"
          inactive-text="手动刷新"
          style="margin-right: 12px;"
        />
        <el-button :icon="Refresh" @click="fetchData" :loading="loading">刷新</el-button>
    </div>

    <el-row :gutter="20">
      <el-col :span="16">
        <el-card shadow="hover" class="trend-card">
          <template #header>
            <span class="card-head-title">{{ getTagName(selectedTag) }} 趋势图</span>
          </template>
          <div class="chart-container">
            <svg ref="chartSvg" class="line-chart" viewBox="0 0 800 300" preserveAspectRatio="xMidYMid meet">
              <!-- Y轴网格线 -->
              <line v-for="i in 5" :key="'grid-' + i"
                :x1="60" :y1="i * 60"
                :x2="800" :y2="i * 60"
                class="grid-line"
              />
              <!-- Y轴标签 -->
              <text v-for="(label, i) in yAxisLabels" :key="'ylabel-' + i"
                :x="50" :y="i * 60 + 4"
                text-anchor="end" class="axis-label"
              >{{ label }}</text>
              <!-- X轴标签 -->
              <text v-for="(label, i) in xAxisLabels" :key="'xlabel-' + i"
                :x="60 + i * (740 / (xAxisLabels.length - 1))" :y="290"
                text-anchor="middle" class="axis-label"
              >{{ label }}</text>
              <!-- 数据线 -->
              <path
                v-if="chartPath"
                :d="chartPath"
                class="series-line"
              />
              <!-- 数据区域填充 -->
              <path
                v-if="areaPath"
                :d="areaPath"
                fill="url(#gradient)"
                opacity="0.3"
              />
              <!-- 渐变定义 -->
              <defs>
                <linearGradient id="gradient" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" class="gradient-stop-top" />
                  <stop offset="100%" class="gradient-stop-bottom" />
                </linearGradient>
              </defs>
              <!-- 数据点 -->
              <circle
                v-for="(point, index) in chartPoints"
                :key="'point-' + index"
                :cx="point.x"
                :cy="point.y"
                r="4"
                class="series-point"
                @mouseenter="showTooltip(point, $event)"
                @mouseleave="hideTooltip"
              />
            </svg>
            <div v-if="tooltipVisible" class="tooltip" :style="tooltipStyle">
              <div class="tooltip-time">{{ tooltipData.time }}</div>
              <div class="tooltip-value">{{ tooltipData.value }} {{ getUnit(selectedTag) }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover" class="realtime-card-container">
          <template #header>
            <span class="card-head-title">实时数值</span>
          </template>
          <div class="realtime-cards">
            <div v-for="tag in availableTags" :key="tag" class="realtime-card">
              <div class="realtime-content">
                <span class="realtime-label">{{ getTagName(tag) }}</span>
                <span class="realtime-value">{{ getRealtimeValue(tag) }}</span>
                <span class="realtime-unit">{{ getUnit(tag) }}</span>
                <div class="realtime-trend" :class="getTrendClass(tag)">
                  <el-icon v-if="getTrend(tag) > 0"><ArrowUp /></el-icon>
                  <el-icon v-else-if="getTrend(tag) < 0"><ArrowDown /></el-icon>
                  <span>{{ getTrendText(tag) }}</span>
                </div>
              </div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px;">
      <el-col :span="16">
        <el-card shadow="hover" class="table-card">
          <template #header>
            <span>历史数据</span>
            <span style="float: right;">
              <template v-if="isSuperAdmin">
                <el-button
                  type="danger"
                  size="small"
                  :disabled="historySelection.length === 0"
                  @click="handleDeleteSelected"
                >删除选中({{ historySelection.length }})</el-button>
                <el-button
                  type="danger"
                  size="small"
                  plain
                  :disabled="historyTotal === 0"
                  @click="handleDeleteAll"
                >清空全部</el-button>
              </template>
              <el-button
                :icon="Refresh"
                size="small"
                @click="fetchHistoryData"
                :loading="historyLoading"
              >刷新</el-button>
            </span>
          </template>
          <!-- 历史表不使用 v-loading：5s 轮询每次置 loading 会导致蓝色转圈闪烁 -->
          <el-table :data="historyData" stripe border @selection-change="onHistorySelectionChange">
            <el-table-column v-if="isSuperAdmin" type="selection" width="42" align="center" />
            <el-table-column prop="id" label="ID" width="80" align="center" />
            <el-table-column prop="msg_id" label="消息ID" min-width="120" show-overflow-tooltip />
            <el-table-column prop="ts" label="上报时间" width="180" align="center">
              <template #default="{ row }">
                {{ formatDate(row.ts) }}
              </template>
            </el-table-column>
            <el-table-column v-for="tag in availableTags" :key="tag" :label="getTagName(tag)" width="120" align="center">
              <template #default="{ row }">
                <span v-if="row.data && row.data[tag] !== undefined">{{ row.data[tag] }} {{ getUnit(tag) }}</span>
                <span v-else class="cell-dash">-</span>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="historyData.length === 0 && !historyLoading" class="empty-tip">
            {{ historyLoading ? '加载中...' : '暂无数据' }}
          </div>
          <div v-if="historyTotal > historyPageSize" class="pager-wrap">
            <el-pagination
              v-model:current-page="historyCurrentPage"
              :page-size="historyPageSize"
              :total="historyTotal"
              layout="prev, pager, next"
              @current-change="handleHistoryPageChange"
            />
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover" class="stat-card">
          <template #header>
            <span class="card-head-title">统计信息</span>
          </template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="最大值">
              <span class="stat-highlight">{{ stats.max?.toFixed(2) || '-' }}</span>
              <span class="stat-unit">{{ getUnit(selectedTag) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="最小值">
              <span class="stat-highlight stat-min">{{ stats.min?.toFixed(2) || '-' }}</span>
              <span class="stat-unit">{{ getUnit(selectedTag) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="平均值">
              <span class="stat-highlight stat-avg">{{ stats.avg?.toFixed(2) || '-' }}</span>
              <span class="stat-unit">{{ getUnit(selectedTag) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="数据点数">
              <span class="stat-count">{{ stats.count || 0 }}</span>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
        <el-card shadow="hover" class="device-info-card">
          <template #header>
            <span class="card-head-title">设备信息</span>
          </template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="设备状态">
              <el-tag :type="deviceStatusType">{{ deviceStatus }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="设备版本">{{ deviceVersion }}</el-descriptions-item>
            <el-descriptions-item label="初次上线">{{ firstOnlineTime }}</el-descriptions-item>
            <el-descriptions-item label="最后活跃">{{ lastActiveTime }}</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
    </el-row>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Refresh, ArrowUp, ArrowDown } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getDeviceData, getDeviceTags, getDevice, deleteDeviceData, type DeviceDataPoint, type DeviceTag, type Device } from '@/api/device'
import { getProjectTags, type ProjectTag } from '@/api/project'
import { useAuthStore } from '@/stores/auth'
import { formatTs as fmtTs, formatTime as fmtTime } from '@/utils/datetime'

const route = useRoute()
const deviceId = String(route.params.deviceId)
const authStore = useAuthStore()
// 数据删除仅对超级管理员开放（与后端路由级 RequireRole 一致）
const isSuperAdmin = computed(() => authStore.role === 'super_admin')

const dataPoints = ref<DeviceDataPoint[]>([])
const deviceTags = ref<DeviceTag[]>([])
const projectTags = ref<ProjectTag[]>([])
// 从上报数据中实际出现过的标签键（在项目/设备标签未配置时作为兜底，避免硬编码）
const dataTagKeys = ref<string[]>([])
const deviceInfo = ref<Device | null>(null)
const loading = ref(false)
const autoRefresh = ref(true)
const timeRange = ref('1h')
const selectedTag = ref('')

// 可选标签：合并设备标签与项目标签（去重），完全动态生成，不做硬编码。
const availableTags = computed(() => {
  const keys: string[] = []
  for (const dt of deviceTags.value) {
    if (dt.tag_key && !keys.includes(dt.tag_key)) keys.push(dt.tag_key)
  }
  for (const pt of projectTags.value) {
    if (pt.tag_key && !keys.includes(pt.tag_key)) keys.push(pt.tag_key)
  }
  // 兜底：标签配置里没有，但上报数据里实际出现过的 key
  for (const k of dataTagKeys.value) {
    if (!keys.includes(k)) keys.push(k)
  }
  return keys
})

// 按时间升序排列的数据点（用于绘图，后端返回为 ts desc）
const orderedPoints = computed(() =>
  [...dataPoints.value].sort((a, b) => a.ts - b.ts)
)

// 时间范围（毫秒）
const rangeMs = computed(() => {
  switch (timeRange.value) {
    case '6h': return 6 * 60 * 60 * 1000
    case '24h': return 24 * 60 * 60 * 1000
    case '1h':
    default: return 60 * 60 * 1000
  }
})

// 计算设备状态
const deviceStatus = computed(() => {
  if (!deviceInfo.value) return '未知'
  switch (deviceInfo.value.status) {
    case 1: return '在线'
    case 0: return '离线'
    case 2: return '未激活'
    default: return '未知'
  }
})

// 计算设备状态类型
const deviceStatusType = computed(() => {
  if (!deviceInfo.value) return 'info'
  switch (deviceInfo.value.status) {
    case 1: return 'success'
    case 0: return 'danger'
    case 2: return 'warning'
    default: return 'info'
  }
})

// 计算最后活跃时间（最新数据的时间）
const lastActiveTime = computed(() => {
  if (dataPoints.value.length === 0) return '-'
  const latestData = dataPoints.value[0]
  return formatDate(latestData.ts)
})

// 计算设备版本（从最新数据中获取）
const deviceVersion = computed(() => {
  if (dataPoints.value.length === 0) return '-'
  // 后端按 ts desc 排序，索引 0 是最新数据
  const latestData = dataPoints.value[0]
  return latestData.version || '-'
})

// 计算初次上线时间（优先使用设备端上报的首次开机时间，否则从历史数据推断）
const firstOnlineTime = computed(() => {
  // 优先使用设备端上报的首次开机时间
  if (deviceInfo.value && deviceInfo.value.first_ts > 0) {
    return formatDate(deviceInfo.value.first_ts)
  }
  // 否则从历史数据推断
  if (dataPoints.value.length === 0) return '-'
  // 后端按 ts desc 排序，最后一个元素是最旧数据
  const oldestData = dataPoints.value[dataPoints.value.length - 1]
  return formatDate(oldestData.ts)
})

const tooltipVisible = ref(false)
const tooltipData = ref({ time: '', value: '' })
const tooltipStyle = ref({})

const stats = ref({
  max: null as number | null,
  min: null as number | null,
  avg: null as number | null,
  count: 0
})

let timer: ReturnType<typeof setInterval> | null = null

function getUnit(tag: string): string {
  // 优先项目标签，其次设备标签；查不到返回空（不再硬编码单位）
  const pt = projectTags.value.find(p => p.tag_key === tag)
  if (pt && pt.unit) {
    return pt.unit
  }
  const dt = deviceTags.value.find(d => d.tag_key === tag)
  return dt?.unit || ''
}

function getTagName(tag: string): string {
  // 优先项目标签 tag_name，其次设备标签 name；都没有则回退英文 tag_key
  const pt = projectTags.value.find(p => p.tag_key === tag)
  if (pt && pt.tag_name) {
    return pt.tag_name
  }
  const dt = deviceTags.value.find(d => d.tag_key === tag)
  return dt?.name || tag
}

function formatTs(ts: number): string {
  return fmtTime(ts)
}

function formatDate(ts: number): string {
  return fmtTs(ts)
}

async function fetchTags() {
  try {
    // 获取设备标签
    const deviceRes = await getDeviceTags(deviceId)
    deviceTags.value = deviceRes.data || []

    // 获取设备信息以获取项目ID
    const deviceInfoRes = await getDevice(deviceId)
    deviceInfo.value = deviceInfoRes.data
    if (deviceInfo.value && deviceInfo.value.project_id) {
      // 获取项目标签
      const projectId = deviceInfo.value.project_id
      const projectRes = await getProjectTags(projectId)
      projectTags.value = projectRes.data || []
    }

    // 设置默认选中的标签
    if (availableTags.value.length > 0 && !selectedTag.value) {
      selectedTag.value = availableTags.value[0]
    }
  } catch {
    // 如果获取失败，使用默认标签
    if (!selectedTag.value) {
      selectedTag.value = 'temperature'
    }
  }
}

async function fetchData() {
  loading.value = true
  try {
    const start = Date.now() - rangeMs.value
    const res = await getDeviceData(deviceId, { start, limit: 500, offset: 0 })
    dataPoints.value = res.data?.list || []
    // 从实际上报数据里收集出现过的标签键作为动态兜底
    const keys = new Set<string>(dataTagKeys.value)
    for (const p of dataPoints.value) {
      const obj = typeof p.data === 'string' ? JSON.parse(p.data) : p.data
      Object.keys(obj || {}).forEach(k => keys.add(k))
    }
    dataTagKeys.value = Array.from(keys)
    calculateStats()
  } catch {
  } finally {
    loading.value = false
  }
}

function calculateStats() {
  const values = getSelectedTagValues()
  if (values.length === 0) {
    stats.value = { max: null, min: null, avg: null, count: 0 }
    return
  }
  stats.value = {
    max: Math.max(...values),
    min: Math.min(...values),
    avg: values.reduce((a, b) => a + b, 0) / values.length,
    count: values.length
  }
}

function getSelectedTagValues(): number[] {
  return orderedPoints.value
    .map(p => {
      const data = typeof p.data === 'string' ? JSON.parse(p.data) : p.data
      return data[selectedTag.value]
    })
    .filter(v => typeof v === 'number')
}

const chartPoints = computed(() => {
  const values = getSelectedTagValues()
  if (values.length === 0) return []
  
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = max - min || 1
  
  const pts = orderedPoints.value
  return pts
    .map((p, index) => {
      const data = typeof p.data === 'string' ? JSON.parse(p.data) : p.data
      const value = data[selectedTag.value]
      if (typeof value !== 'number') return null
      
      return {
        x: 60 + (pts.length > 1 ? (index / (pts.length - 1)) * 740 : 0),
        y: 240 - ((value - min) / range) * 200,
        value,
        ts: p.ts
      }
    })
    .filter((p): p is { x: number; y: number; value: number; ts: number } => p !== null)
})

const chartPath = computed(() => {
  if (chartPoints.value.length < 2) return ''
  return chartPoints.value
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`)
    .join(' ')
})

const areaPath = computed(() => {
  if (chartPoints.value.length < 2) return ''
  const linePath = chartPoints.value
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`)
    .join(' ')
  const first = chartPoints.value[0]
  const last = chartPoints.value[chartPoints.value.length - 1]
  return `${linePath} L ${last.x} 240 L ${first.x} 240 Z`
})

const yAxisLabels = computed(() => {
  const values = getSelectedTagValues()
  if (values.length === 0) return ['0', '0', '0', '0', '0']
  
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = max - min || 1
  
  return [0, 1, 2, 3, 4].map(i => {
    const value = max - (i / 4) * range
    return value.toFixed(1)
  })
})

const xAxisLabels = computed(() => {
  if (orderedPoints.value.length === 0) return []
  
  const pts = orderedPoints.value
  const step = Math.max(1, Math.floor(pts.length / 5))
  return pts
    .filter((_, i) => i % step === 0 || i === pts.length - 1)
    .map(p => formatTs(p.ts))
})

const realtimeValues = ref<Record<string, number>>({})
const prevValues = ref<Record<string, number>>({})

function getRealtimeValue(tag: string): string {
  const values = orderedPoints.value
    .map(p => {
      const data = typeof p.data === 'string' ? JSON.parse(p.data) : p.data
      return data[tag]
    })
    .filter(v => typeof v === 'number')
  
  if (values.length === 0) return '-'
  
  const latest = values[values.length - 1]
  realtimeValues.value[tag] = latest
  
  if (prevValues.value[tag] === undefined) {
    prevValues.value[tag] = latest
  }
  
  return latest.toFixed(2)
}

function getTrend(tag: string): number {
  const current = realtimeValues.value[tag]
  const prev = prevValues.value[tag]
  if (current === undefined || prev === undefined) return 0
  return current - prev
}

function getTrendClass(tag: string): string {
  const trend = getTrend(tag)
  if (trend > 0) return 'trend-up'
  if (trend < 0) return 'trend-down'
  return 'trend-neutral'
}

function getTrendText(tag: string): string {
  const trend = getTrend(tag)
  if (trend > 0) return `+${trend.toFixed(2)}`
  if (trend < 0) return trend.toFixed(2)
  return '0'
}

function showTooltip(point: { value: number; ts: number }, event: MouseEvent) {
  tooltipData.value = {
    time: formatDate(point.ts),
    value: point.value.toFixed(2)
  }
  tooltipStyle.value = {
    left: `${event.clientX + 10}px`,
    top: `${event.clientY + 10}px`
  }
  tooltipVisible.value = true
}

function hideTooltip() {
  tooltipVisible.value = false
}

function startPolling() {
  stopPolling()
  timer = setInterval(() => {
    fetchData()
    // 只在第一页时刷新历史数据，避免频繁翻页时重复请求
    if (historyCurrentPage.value === 1) {
      fetchHistoryData()
    }
  }, 5000)
}

// 历史数据相关
const historyData = ref<DeviceDataPoint[]>([])
const historyTotal = ref(0)
const historyCurrentPage = ref(1)
const historyPageSize = 20
const historyLoading = ref(false)

function formatHistoryTs(ts: number): string {
  return fmtTs(ts)
}

async function fetchHistoryData() {
  historyLoading.value = true
  try {
    const offset = (historyCurrentPage.value - 1) * historyPageSize
    const res = await getDeviceData(deviceId, { limit: historyPageSize, offset })
    historyData.value = res.data?.list || []
    historyTotal.value = res.data?.total || 0
  } catch {
  } finally {
    historyLoading.value = false
  }
}

function handleHistoryPageChange(page: number) {
  historyCurrentPage.value = page
  fetchHistoryData()
}

// ===== 数据删除（仅超管）=====
const historySelection = ref<DeviceDataPoint[]>([])
const deleting = ref(false)

function onHistorySelectionChange(rows: DeviceDataPoint[]) {
  historySelection.value = rows
}

async function doDelete(payload: { ids?: number[]; all?: boolean }, tip: string) {
  deleting.value = true
  try {
    const res = await deleteDeviceData(deviceId, payload)
    ElMessage.success(`已删除 ${res.data?.deleted ?? 0} 条${tip}`)
    if (historyCurrentPage.value > 1 && historyData.value.length === 0) {
      historyCurrentPage.value = 1
    }
    fetchHistoryData()
    fetchData()
  } catch {
    // 拦截器已提示错误
  } finally {
    deleting.value = false
  }
}

async function handleDeleteSelected() {
  const ids = historySelection.value.map(r => r.id)
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${ids.length} 条数据？删除后不可恢复。`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }
  await doDelete({ ids }, '')
}

async function handleDeleteAll() {
  try {
    await ElMessageBox.confirm(`确定清空设备 ${deviceId} 的全部 ${historyTotal.value}+ 条数据？此操作不可恢复！`, '清空全部', {
      type: 'error',
      confirmButtonText: '全部删除',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }
  await doDelete({ all: true }, '')
}

function stopPolling() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

watch(autoRefresh, (val) => {
  if (val) {
    startPolling()
  } else {
    stopPolling()
  }
})

watch(selectedTag, () => {
  calculateStats()
})

watch(timeRange, () => {
  fetchData()
})

onMounted(async () => {
  await fetchTags()
  fetchData()
  fetchHistoryData()
  if (autoRefresh.value) {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.chart-container {
  position: relative;
  width: 100%;
  height: 320px;
}

.line-chart {
  width: 100%;
  height: 100%;
}

.axis-label {
  font-size: 11px;
  fill: var(--wd-text-placeholder);
}

.grid-line {
  stroke: var(--wd-border-lighter);
  stroke-width: 1;
  stroke-dasharray: 4, 4;
}

.series-line {
  fill: none;
  stroke: var(--wd-primary);
  stroke-width: 2;
}

.gradient-stop-top {
  stop-color: var(--wd-primary);
}

.gradient-stop-bottom {
  stop-color: #ffffff;
}

.series-point {
  fill: var(--wd-primary);
  cursor: pointer;
  transition: all 0.2s ease;
}

.series-point:hover {
  r: 6;
  filter: drop-shadow(0 0 6px var(--wd-primary-bg, rgba(64, 158, 255, 0.5)));
}

.tooltip {
  position: fixed;
  background: #23272f;
  color: #f5f7fa;
  padding: 12px 16px;
  border-radius: 10px;
  font-size: 13px;
  z-index: 1000;
  pointer-events: none;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.tooltip-time {
  margin-bottom: 8px;
  opacity: 0.7;
  font-size: 12px;
}

.tooltip-value {
  font-size: 18px;
  font-weight: 600;
}

.realtime-card-container {
  height: 420px;
}

.realtime-card-container :deep(.el-card__body) {
  padding: 16px;
  height: calc(100% - 57px);
  overflow: hidden;
}

.realtime-cards {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  height: 100%;
  overflow-y: auto;
  padding-right: 8px;
  align-content: start;
}

.realtime-cards::-webkit-scrollbar {
  width: 4px;
}

.realtime-cards::-webkit-scrollbar-track {
  background: transparent;
}

.realtime-cards::-webkit-scrollbar-thumb {
  background: var(--wd-border);
  border-radius: 2px;
}

.realtime-cards::-webkit-scrollbar-thumb:hover {
  background: var(--wd-text-placeholder);
}

.realtime-card {
  background: var(--wd-surface);
  height: 64px;
  padding: 10px 12px;
  border-radius: var(--wd-radius-lg);
  box-shadow: var(--wd-shadow-sm);
  border: 1px solid var(--wd-border-lighter);
  transition: all 0.3s ease;
  position: relative;
  overflow: hidden;
  box-sizing: border-box;
}

.realtime-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  background: linear-gradient(180deg, var(--wd-primary) 0%, var(--wd-primary-light) 100%);
}

.realtime-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--wd-shadow-md);
}

.realtime-content {
  display: grid;
  grid-template-columns: minmax(36px, 52px) minmax(0, 1fr) auto;
  grid-template-areas:
    "label value unit"
    "label trend trend";
  align-items: center;
  column-gap: 6px;
  row-gap: 4px;
  height: 100%;
}

.realtime-label {
  grid-area: label;
  font-size: 12px;
  color: var(--wd-text-regular);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.realtime-value {
  grid-area: value;
  font-size: 18px;
  font-weight: 700;
  color: var(--wd-text-primary);
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.realtime-unit {
  grid-area: unit;
  font-size: 12px;
  color: var(--wd-text-secondary);
  font-weight: 500;
  white-space: nowrap;
}

.realtime-trend {
  grid-area: trend;
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: 2px;
  font-weight: 500;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--wd-bg);
  width: fit-content;
  max-width: 100%;
  overflow: hidden;
  white-space: nowrap;
}

.realtime-badge {
  font-size: 10px;
  color: #fff;
  background: var(--wd-success);
  padding: 2px 6px;
  border-radius: 10px;
  font-weight: 500;
  margin-left: auto;
}

.trend-up {
  color: var(--wd-success);
}

.trend-down {
  color: var(--wd-danger);
}

.trend-neutral {
  color: var(--wd-text-secondary);
}

.card-head-title {
  font-weight: 600;
  color: var(--wd-text-primary);
}

.cell-dash {
  color: var(--wd-text-secondary);
}

.pager-wrap {
  display: flex;
  justify-content: center;
  margin-top: 16px;
}

.stat-unit {
  color: var(--wd-text-secondary);
  margin-left: 8px;
}

.stat-min {
  color: var(--wd-success);
}

.stat-avg {
  color: #722ed1;
}

.stat-count {
  font-size: 16px;
  font-weight: 600;
  color: var(--wd-text-primary);
}

.stat-highlight {
  font-size: 20px;
  font-weight: 700;
  color: var(--wd-primary);
}

.table-card {
  border-radius: var(--wd-radius-lg);
  overflow: hidden;
  background: var(--wd-surface);
}

.table-card :deep(.el-card__header) {
  background: var(--wd-surface);
  border-bottom: 1px solid var(--wd-border-lighter);
}

.table-card :deep(.el-card__body) {
  background: var(--wd-surface);
}

.table-card :deep(.el-table) {
  border-radius: 0;
  background: var(--wd-surface);
}

.table-card :deep(.el-table th) {
  background: var(--wd-surface);
  font-weight: 600;
  color: var(--wd-text-regular);
  border-bottom: 1px solid var(--wd-border-lighter);
}

.table-card :deep(.el-table td) {
  background: var(--wd-surface);
  border-bottom: 1px solid var(--wd-border-lighter);
}

.stat-card :deep(.el-card__body) {
  padding: 16px;
}

.stat-card :deep(.el-descriptions-item__label) {
  color: var(--wd-text-regular);
  font-weight: 500;
}

.stat-card :deep(.el-descriptions-item__content) {
  color: var(--wd-text-primary);
}

.device-info-card :deep(.el-card__body) {
  padding: 16px;
}

.device-info-card :deep(.el-tag) {
  font-weight: 500;
}
</style>