<template>
  <div class="page-container">
    <div class="toolbar">
      <div class="toolbar-left">
        <el-input
          v-model="filterDeviceId"
          placeholder="按设备ID筛选"
          clearable
          style="width: 220px;"
          @clear="onFilterChange"
          @keyup.enter="onFilterChange"
        />
        <el-select v-model="filterStatus" placeholder="状态" clearable style="width: 140px; margin-left: 8px;" @change="onFilterChange">
          <el-option label="已送达" value="delivered" />
          <el-option label="执行成功" value="success" />
          <el-option label="执行失败" value="failed" />
          <el-option label="超时" value="timeout" />
          <el-option label="下发中" value="pending" />
        </el-select>
        <el-date-picker
          v-model="timeRange"
          type="datetimerange"
          range-separator="至"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          style="margin-left: 8px;"
          @change="onFilterChange"
        />
        <el-button type="primary" style="margin-left: 8px;" @click="onFilterChange">查询</el-button>
      </div>
      <div class="toolbar-right">
        <el-switch v-model="autoRefresh" active-text="自动刷新" inactive-text="手动刷新" style="margin-right: 12px;" />
        <el-button @click="fetchLogs" :loading="loading">刷新</el-button>
        <el-button type="success" :loading="exporting" @click="handleExport">
          <el-icon><Download /></el-icon>&nbsp;导出CSV
        </el-button>
        <template v-if="isSuperAdmin">
          <el-button type="danger" plain :disabled="selectedRows.length === 0" @click="handleDeleteSelected">
            删除选中<template v-if="selectedRows.length">({{ selectedRows.length }})</template>
          </el-button>
          <el-button type="danger" @click="handleClearFilter">
            {{ hasActiveFilter ? '清空筛选日志' : '清空全部日志' }}
          </el-button>
        </template>
      </div>
    </div>

    <el-card class="table-card">
      <el-table :data="filtered" v-loading="loading" stripe border max-height="calc(100vh - 300px)" @selection-change="onSelectionChange">
        <el-table-column v-if="isSuperAdmin" type="selection" width="42" align="center" />
        <el-table-column prop="id" label="ID" width="80" align="center" />
        <el-table-column prop="device_id" label="设备ID" width="160" />
        <el-table-column prop="msg_id" label="消息ID" width="150" show-overflow-tooltip />
        <el-table-column label="控制标签" min-width="180">
          <template #default="{ row }">
            <pre class="tags-pre">{{ formatTags(row.tags) }}</pre>
          </template>
        </el-table-column>
        <el-table-column label="执行状态" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="响应码" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.ack_code != null" :type="row.ack_code === 0 ? 'success' : 'danger'" size="small">
              {{ row.ack_code }}
            </el-tag>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column prop="ack_msg" label="响应消息" width="160" show-overflow-tooltip />
        <el-table-column label="创建时间" width="180" align="center">
          <template #default="{ row }">{{ formatTs(row.created_at) }}</template>
        </el-table-column>
      </el-table>
      <div v-if="filtered.length === 0 && !loading" class="empty-tip">
        暂无控制日志
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
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Download } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getControlLogs, exportControlLogsCsv, deleteControlLogs, type ControlLog, type ControlLogQuery } from '@/api/firmware'
import { formatTs } from '@/utils/datetime'
import { useAuthStore } from '@/stores/auth'

// 支持从设备控制页跳转时预填设备过滤（/logs/control?device_id=xxx）
const route = useRoute()
const authStore = useAuthStore()
const isSuperAdmin = computed(() => authStore.role === 'super_admin')
const selectedRows = ref<ControlLog[]>([])

function onSelectionChange(rows: ControlLog[]) {
  selectedRows.value = rows
}

// 后端过滤条件（设备 + 时间范围）；状态过滤在前端完成，不计入删除范围
const hasActiveFilter = computed(() => !!filterDeviceId.value || !!timeRange.value)

const logs = ref<ControlLog[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = 50
const loading = ref(false)
const exporting = ref(false)
const autoRefresh = ref(false)
const filterDeviceId = ref('')
const filterStatus = ref('')
const timeRange = ref<[Date, Date] | null>(null)

let timer: ReturnType<typeof setInterval> | null = null

function formatTags(tags: Record<string, unknown>): string {
  return typeof tags === 'string' ? tags : JSON.stringify(tags, null, 2)
}

function statusType(status?: string) {
  return ({ success: 'success', failed: 'danger', timeout: 'info', delivered: 'warning', pending: 'warning' } as Record<string, 'success' | 'danger' | 'info' | 'warning'>)[status || 'pending']
}
function statusText(status?: string) {
  return ({ success: '执行成功', failed: '执行失败', timeout: '超时', delivered: '已送达', pending: '下发中' } as Record<string, string>)[status || 'pending'] || status || '下发中'
}

function buildQuery(): ControlLogQuery {
  const offset = (currentPage.value - 1) * pageSize
  const q: ControlLogQuery = { device_id: filterDeviceId.value || undefined, limit: pageSize, offset }
  if (timeRange.value && timeRange.value.length === 2) {
    q.start = timeRange.value[0].getTime()
    q.end = timeRange.value[1].getTime()
  }
  return q
}

async function fetchLogs() {
  loading.value = true
  try {
    const res = await getControlLogs(buildQuery())
    logs.value = res.data?.list || []
    total.value = res.data?.total || 0
  } catch {
  } finally {
    loading.value = false
    selectedRows.value = []
  }
}

// 状态过滤在前端完成（控制日志量可控；后端已按设备/时间/租户过滤）
const filtered = computed(() => {
  if (!filterStatus.value) return logs.value
  return logs.value.filter(l => l.status === filterStatus.value)
})

function onFilterChange() {
  currentPage.value = 1
  fetchLogs()
}

function handlePageChange(page: number) {
  currentPage.value = page
  fetchLogs()
}

async function handleExport() {
  exporting.value = true
  try {
    const q = buildQuery()
    q.limit = 100000
    q.offset = 0
    await exportControlLogsCsv(q)
    ElMessage.success('导出成功')
  } catch {
  } finally {
    exporting.value = false
  }
}

async function handleDeleteSelected() {
  const ids = selectedRows.value.map(r => r.id)
  if (ids.length === 0) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${ids.length} 条控制日志吗？此操作不可恢复。`, '确认删除', { type: 'warning' })
  } catch {
    return
  }
  try {
    const res = await deleteControlLogs({ ids })
    ElMessage.success(`已删除 ${res.data?.deleted ?? ids.length} 条日志`)
    // 本页删空且非首页时回退一页，避免停留在空页
    if (filtered.value.length === ids.length && currentPage.value > 1) currentPage.value--
    fetchLogs()
  } catch {
  }
}

async function handleClearFilter() {
  const q = buildQuery()
  const statusNote = filterStatus.value ? '（注意：执行状态筛选仅前端生效，将删除该设备/时间范围内所有状态的日志）' : ''
  const msg = hasActiveFilter.value
    ? `将删除当前筛选条件下的全部控制日志（约 ${total.value} 条，包含未翻页部分），此操作不可恢复，是否继续？${statusNote}`
    : '将清空【所有租户】的全部控制日志，此操作不可恢复，是否继续？'
  try {
    await ElMessageBox.confirm(msg, '危险操作', { type: 'warning', confirmButtonText: '确认清空', confirmButtonClass: 'el-button--danger' })
  } catch {
    return
  }
  try {
    const res = await deleteControlLogs({
      all: true,
      device_id: q.device_id,
      start: q.start,
      end: q.end
    })
    ElMessage.success(`已删除 ${res.data?.deleted ?? 0} 条日志`)
    currentPage.value = 1
    fetchLogs()
  } catch {
  }
}

function startPolling() {
  stopPolling()
  timer = setInterval(fetchLogs, 5000)
}
function stopPolling() {
  if (timer) { clearInterval(timer); timer = null }
}

watch(autoRefresh, (val) => { if (val) startPolling(); else stopPolling() })
onMounted(() => {
  const qDevice = route.query.device_id
  if (typeof qDevice === 'string' && qDevice) {
    filterDeviceId.value = qDevice
    currentPage.value = 1
  }
  fetchLogs()
})
onUnmounted(stopPolling)
</script>

<style scoped>
.tags-pre {
  margin: 0;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
