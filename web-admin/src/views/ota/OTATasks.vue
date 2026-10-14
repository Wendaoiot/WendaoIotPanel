<template>
  <div class="page-container">
    <div class="toolbar">
      <div class="toolbar-right">
        <el-button type="primary" @click="showTaskDialog">
          <el-icon><Upload /></el-icon>创建升级任务
        </el-button>
      </div>
    </div>

    <el-card class="table-card" style="margin-bottom: 20px;">
      <el-table :data="tasks" v-loading="loading" stripe border @row-click="selectTask">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="firmware_id" label="固件ID" width="80" />
        <el-table-column label="目标类型" width="100">
          <template #default="{ row }">{{ row.target_type === 'device' ? '单设备' : '项目级' }}</template>
        </el-table-column>
        <el-table-column prop="target_id" label="目标" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="tagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="170" />
        <el-table-column label="操作" width="80" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="info" @click.stop="selectTask(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card v-if="selectedTaskId" class="table-card">
      <template #header>
        <div class="logs-header">
          <span>升级日志 (任务ID: {{ selectedTaskId }})</span>
          <span v-if="isSuperAdmin" class="logs-actions">
            <el-button size="small" type="danger" plain :disabled="selectedLogs.length === 0" @click="handleDeleteSelectedLogs">
              删除选中<template v-if="selectedLogs.length">({{ selectedLogs.length }})</template>
            </el-button>
            <el-button size="small" type="danger" @click="handleClearTaskLogs">清空本任务日志</el-button>
          </span>
        </div>
      </template>
      <el-table :data="logs" v-loading="logsLoading" stripe border size="small" @selection-change="onLogSelectionChange">
        <el-table-column v-if="isSuperAdmin" type="selection" width="42" align="center" />
        <el-table-column prop="id" label="ID" width="70" align="center" />
        <el-table-column prop="device_id" label="设备ID" width="160" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="logTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="进度" width="120">
          <template #default="{ row }">
            <el-progress :percentage="row.progress" :status="row.status === 'failed' ? 'exception' : undefined" />
          </template>
        </el-table-column>
        <el-table-column prop="error_msg" label="错误信息" show-overflow-tooltip />
        <el-table-column prop="updated_at" label="更新时间" width="170" />
        <el-table-column v-if="isSuperAdmin" label="操作" width="80" align="center" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" link @click="handleDeleteLog(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" title="创建OTA升级任务" width="500px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="固件" prop="firmware_id">
          <el-select v-model="form.firmware_id" placeholder="选择固件" style="width: 100%">
            <el-option v-for="fw in firmwares" :key="fw.id" :label="`${fw.name} v${fw.version}`" :value="fw.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="目标类型" prop="target_type">
          <el-radio-group v-model="form.target_type">
            <el-radio value="device">单设备</el-radio>
            <el-radio value="project">项目(全部设备)</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="form.target_type === 'device' ? '设备ID' : '项目ID'" prop="target_id">
          <el-input v-model="form.target_id" :placeholder="form.target_type === 'device' ? '如: ESP32-001' : '项目ID数字'" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleCreate">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { getFirmwares, getOTATasks, getOTALogs, createOTATask, deleteOTALogs } from '@/api/firmware'
import type { Firmware, OTATask, OTALog } from '@/api/firmware'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()
const isSuperAdmin = computed(() => authStore.role === 'super_admin')
const selectedLogs = ref<OTALog[]>([])

function onLogSelectionChange(rows: OTALog[]) {
  selectedLogs.value = rows
}

const tasks = ref<OTATask[]>([])
const firmwares = ref<Firmware[]>([])
const logs = ref<OTALog[]>([])
const loading = ref(false)
const logsLoading = ref(false)
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref<FormInstance>()
const selectedTaskId = ref(0)

let logsTimer: ReturnType<typeof setInterval> | null = null

const form = reactive({ firmware_id: undefined as number | undefined, target_type: 'device', target_id: '' })

const rules: FormRules = {
  firmware_id: [{ required: true, message: '请选择固件', trigger: 'change' }],
  target_type: [{ required: true }],
  target_id: [{ required: true, message: '请输入目标ID', trigger: 'blur' }]
}

function tagType(status: string) {
  if (status === 'running') return 'warning'
  if (status === 'done') return 'success'
  return 'info'
}

function logTagType(status: string) {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'downloading' || status === 'installing') return 'warning'
  return 'info'
}

async function fetchTasks() { loading.value = true; try { tasks.value = (await getOTATasks()).data || [] } catch { } finally { loading.value = false } }
async function fetchFirmwares() { try { firmwares.value = (await getFirmwares()).data || [] } catch { } }

async function selectTask(row: OTATask) {
  selectedTaskId.value = row.id
  await fetchLogs()
  if (logsTimer) clearInterval(logsTimer)
  logsTimer = setInterval(fetchLogs, 3000)
}

async function fetchLogs() {
  if (!selectedTaskId.value) return
  logsLoading.value = true
  try { logs.value = (await getOTALogs(selectedTaskId.value)).data || [] } catch { } finally { logsLoading.value = false; selectedLogs.value = [] }
}

async function handleDeleteLog(row: OTALog) {
  try {
    await ElMessageBox.confirm(`确定删除设备 "${row.device_id}" 在任务 ${selectedTaskId.value} 中的升级日志吗？`, '确认删除', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteOTALogs({ ids: [row.id] })
    ElMessage.success('日志已删除')
    fetchLogs()
  } catch { }
}

async function handleDeleteSelectedLogs() {
  const ids = selectedLogs.value.map(l => l.id)
  if (ids.length === 0) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${ids.length} 条升级日志吗？此操作不可恢复。`, '确认删除', { type: 'warning' })
  } catch {
    return
  }
  try {
    const res = await deleteOTALogs({ ids })
    ElMessage.success(`已删除 ${res.data?.deleted ?? ids.length} 条日志`)
    fetchLogs()
  } catch { }
}

async function handleClearTaskLogs() {
  try {
    await ElMessageBox.confirm(`将清空任务 ${selectedTaskId.value} 下的全部升级日志（仅日志，不删除任务），此操作不可恢复，是否继续？`, '危险操作', {
      type: 'warning', confirmButtonText: '确认清空', confirmButtonClass: 'el-button--danger'
    })
  } catch {
    return
  }
  try {
    const res = await deleteOTALogs({ task_id: selectedTaskId.value })
    ElMessage.success(`已删除 ${res.data?.deleted ?? 0} 条日志`)
    fetchLogs()
  } catch { }
}

function showTaskDialog() { dialogVisible.value = true }
function resetForm() { form.firmware_id = undefined; form.target_id = ''; form.target_type = 'device'; formRef.value?.resetFields() }

async function handleCreate() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    await createOTATask({ firmware_id: form.firmware_id!, target_type: form.target_type, target_id: form.target_id })
    ElMessage.success('OTA升级任务已下发')
    dialogVisible.value = false
    fetchTasks()
  } catch { } finally { submitting.value = false }
}

onMounted(() => { fetchFirmwares(); fetchTasks() })
</script>

<style scoped>
.logs-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.logs-actions {
  display: inline-flex;
  gap: 8px;
}
</style>
