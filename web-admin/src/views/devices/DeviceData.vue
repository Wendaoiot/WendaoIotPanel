<template>
  <div class="tab-pane">
    <div class="tab-toolbar">
        <el-date-picker
          v-model="timeRange"
          type="datetimerange"
          range-separator="至"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          value-format="x"
          style="margin-right: 12px;"
          @change="handleRangeChange"
        />
        <el-switch
          v-model="autoRefresh"
          active-text="自动刷新"
          inactive-text="手动刷新"
          style="margin-right: 12px;"
        />
        <el-button type="success" :loading="exporting" @click="handleExport">
          <el-icon><Download /></el-icon>
          导出CSV
        </el-button>
        <template v-if="isSuperAdmin">
          <el-button type="danger" :disabled="selection.length === 0" @click="handleDeleteSelected">
            删除选中({{ selection.length }})
          </el-button>
          <el-button type="danger" plain :disabled="total === 0" @click="handleDeleteAll">清空全部</el-button>
        </template>
        <el-button :icon="Refresh" @click="fetchData" :loading="loading">刷新</el-button>
    </div>

    <el-card class="table-card">
      <!-- 不用 v-loading：3s 轮询会让蓝色转圈反复闪烁 -->
      <el-table :data="dataPoints" stripe border @selection-change="onSelectionChange">
        <el-table-column v-if="isSuperAdmin" type="selection" width="42" align="center" />
        <el-table-column prop="id" label="ID" width="80" align="center" />
        <el-table-column prop="msg_id" label="消息ID" width="120" show-overflow-tooltip />
        <el-table-column prop="ts" label="时间戳" width="180" align="center">
          <template #default="{ row }">
            {{ formatTs(row.ts) }}
          </template>
        </el-table-column>
        <el-table-column prop="device_ts" label="设备时间" width="180" align="center">
          <template #default="{ row }">
            {{ row.device_ts ? formatTs(row.device_ts) : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="数据" min-width="300">
          <template #default="{ row }">
            <pre class="data-pre">{{ formatData(row.data) }}</pre>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="接收时间" width="180" align="center" />
      </el-table>
      <div v-if="dataPoints.length === 0" class="empty-tip">
        {{ loading ? '加载中...' : '暂无数据' }}
      </div>
      <div v-if="total > pageSize" style="display: flex; justify-content: center; margin-top: 16px;">
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="pageSize"
          :total="total"
          layout="prev, pager, next"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Refresh, Download } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getDeviceData, exportDeviceDataCsv, deleteDeviceData, type DeviceDataPoint } from '@/api/device'
import { useAuthStore } from '@/stores/auth'
import { formatTs } from '@/utils/datetime'

const route = useRoute()
const deviceId = String(route.params.deviceId)
const authStore = useAuthStore()
// 数据删除仅对超级管理员开放（与后端路由级 RequireRole 一致）
const isSuperAdmin = computed(() => authStore.role === 'super_admin')

const dataPoints = ref<DeviceDataPoint[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = 20
const loading = ref(false)
const autoRefresh = ref(true)
const exporting = ref(false)
// datetimerange，value-format="x" → [开始毫秒, 结束毫秒]
const timeRange = ref<[number, number] | null>(null)

let timer: ReturnType<typeof setInterval> | null = null

function formatData(data: Record<string, unknown>): string {
  return JSON.stringify(data, null, 2)
}

function rangeQuery(): { start?: number; end?: number } {
  if (!timeRange.value) return {}
  return { start: Number(timeRange.value[0]), end: Number(timeRange.value[1]) }
}

async function fetchData() {
  loading.value = true
  try {
    const offset = (currentPage.value - 1) * pageSize
    const res = await getDeviceData(deviceId, { ...rangeQuery(), limit: pageSize, offset })
    dataPoints.value = res.data?.list || []
    total.value = res.data?.total || 0
  } catch {
  } finally {
    loading.value = false
  }
}

function handleRangeChange() {
  currentPage.value = 1
  fetchData()
}

async function handleExport() {
  exporting.value = true
  try {
    await exportDeviceDataCsv(deviceId, rangeQuery())
    ElMessage.success('已开始导出CSV')
  } catch {
    // 拦截器已提示错误
  } finally {
    exporting.value = false
  }
}

function handlePageChange(page: number) {
  currentPage.value = page
  fetchData()
}

// ===== 数据删除（仅超管）=====
const selection = ref<DeviceDataPoint[]>([])

function onSelectionChange(rows: DeviceDataPoint[]) {
  selection.value = rows
}

async function doDelete(payload: { ids?: number[]; all?: boolean }) {
  try {
    const res = await deleteDeviceData(deviceId, payload)
    ElMessage.success(`已删除 ${res.data?.deleted ?? 0} 条数据`)
    if (currentPage.value > 1 && dataPoints.value.length === 0) {
      currentPage.value = 1
    }
    fetchData()
  } catch {
    // 拦截器已提示错误
  }
}

async function handleDeleteSelected() {
  const ids = selection.value.map(r => r.id)
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${ids.length} 条数据？删除后不可恢复。`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }
  await doDelete({ ids })
}

async function handleDeleteAll() {
  try {
    await ElMessageBox.confirm(`确定清空设备 ${deviceId} 的全部 ${total.value}+ 条数据？此操作不可恢复！`, '清空全部', {
      type: 'error',
      confirmButtonText: '全部删除',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }
  await doDelete({ all: true })
}

function startPolling() {
  stopPolling()
  timer = setInterval(fetchData, 3000)
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

onMounted(() => {
  fetchData()
  if (autoRefresh.value) {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.data-pre {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
}
</style>
