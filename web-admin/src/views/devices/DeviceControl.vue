<template>
  <div class="tab-pane">
    <el-row :gutter="20">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>下发控制指令</span>
          </template>
          <el-form label-width="100px" @submit.prevent="handleControl">
            <el-form-item label="控制对象" v-for="(item, index) in controlItems" :key="index">
              <div style="display: flex; gap: 8px; width: 100%;">
                <el-input v-model="item.key" placeholder="标签键" style="flex: 1" />
                <el-input-number v-model="item.value" :min="0" :max="65535" placeholder="值" style="width: 120px" />
                <el-button type="danger" circle size="small" @click="removeItem(index)">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </div>
            </el-form-item>
            <el-form-item>
              <el-button @click="addItem">添加控制项</el-button>
              <el-button type="primary" native-type="submit" :loading="sending">发送指令</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card>
          <template #header>
            <div style="display:flex;justify-content:space-between;align-items:center;">
              <span>快捷控制（可自定义命名）</span>
              <el-button text type="primary" @click="openCmdEditor">
                <el-icon><Edit /></el-icon>&nbsp;自定义命令
              </el-button>
            </div>
          </template>

          <el-empty v-if="commands.length === 0" description="暂无自定义命令，点击右上角‘自定义命令’创建" :image-size="60" />
          <div v-else class="quick-controls">
            <el-button
              v-for="btn in commands"
              :key="btn.id"
              :type="btn.danger ? 'danger' : 'primary'"
              plain
              style="margin: 8px;"
              :loading="sendingKey === btn.tag_key"
              @click="quickControl(btn)"
            >
              {{ btn.name }}
            </el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 指令执行结果状态流 -->
    <el-card style="margin-top: 20px;">
        <template #header>
          <span>最近指令执行状态</span>
          <el-tag v-if="lastStatus" :type="statusTagType(lastStatus.status)" style="margin-left: 12px;">
            {{ statusText(lastStatus) }}
          </el-tag>
        </template>
        <div v-if="pendingMsg" class="pending-hint">指令发送中，等待设备响应…（消息ID：{{ pendingMsg }}）</div>
        <el-empty v-else-if="!lastStatus" description="尚未发送指令" :image-size="60" />
        <el-descriptions v-else :column="2" border size="small">
          <el-descriptions-item label="消息ID">{{ lastStatus.msg_id }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="statusTagType(lastStatus.status)">{{ statusText(lastStatus) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="响应码">{{ lastStatus.ack_code ?? '—' }}</el-descriptions-item>
          <el-descriptions-item label="设备响应">{{ lastStatus.ack_msg || '—' }}</el-descriptions-item>
        </el-descriptions>
    </el-card>

    <!-- 该设备的控制日志（历史指令执行记录） -->
    <el-card style="margin-top: 20px;">
      <template #header>
        <div style="display:flex; align-items:center;">
          <span>该设备的控制日志</span>
          <el-button text size="small" style="margin-left:12px;" @click="refreshDeviceLogs">
            <el-icon><Refresh /></el-icon>&nbsp;刷新
          </el-button>
          <el-button text size="small" type="primary" style="margin-left:auto;"
            @click="$router.push({ path: '/control-logs', query: { device_id: deviceId } })">
            查看全部
          </el-button>
        </div>
      </template>
      <el-table :data="deviceLogs" size="small" v-loading="logsLoading">
        <el-table-column label="下发时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="控制内容" min-width="180">
          <template #default="{ row }">{{ tagsSummary(row.tags) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="130">
          <template #default="{ row }">
            <el-tag :type="logStatusTagType(row.status)" size="small">{{ logStatusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="响应码" width="80">
          <template #default="{ row }">{{ row.ack_code ?? '—' }}</template>
        </el-table-column>
        <el-table-column label="设备响应" min-width="140">
          <template #default="{ row }">{{ row.ack_msg || '—' }}</template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 自定义命令编辑对话框 -->
    <el-dialog v-model="cmdDialog" title="自定义控制命令" width="560px">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px;"
        title="命令保存在项目维度；‘危险命令’（重启/恢复出厂等）下发前会二次确认。" />
      <el-table :data="commands" size="small" style="margin-bottom:12px;">
        <el-table-column prop="name" label="按钮名称" />
        <el-table-column prop="tag_key" label="标签" width="120" />
        <el-table-column prop="value" label="值" width="70" />
        <el-table-column label="危险" width="70">
          <template #default="{ row }">
            <el-tag v-if="row.danger" type="danger" size="small">是</el-tag>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-button text type="danger" size="small" @click="removeCommand(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-form label-width="90px">
        <el-form-item label="按钮名称"><el-input v-model="newCmd.name" placeholder="如：开继电器1" /></el-form-item>
        <el-form-item label="标签键"><el-input v-model="newCmd.tag_key" placeholder="如 relay1" /></el-form-item>
        <el-form-item label="值"><el-input-number v-model="newCmd.value" :min="0" :max="65535" /></el-form-item>
        <el-form-item label="危险命令"><el-switch v-model="newCmd.danger" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cmdDialog = false">关闭</el-button>
        <el-button type="primary" @click="addCommand">添加命令</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onUnmounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Refresh } from '@element-plus/icons-vue'
import {
  sendDeviceControl, getControlStatus, getDevice,
  listCommands, createCommand, deleteCommand,
  type ControlStatus, type ControlCommand
} from '@/api/device'
import { getControlLogs, type ControlLog } from '@/api/firmware'

const route = useRoute()
const deviceId = String(route.params.deviceId)

const sending = ref(false)
const sendingKey = ref('')
const pendingMsg = ref('')
const lastStatus = ref<ControlStatus | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null
let projectId = 0

interface ControlItem { key: string; value: number }
const controlItems = reactive<ControlItem[]>([{ key: 'relay1', value: 0 }])

const commands = ref<ControlCommand[]>([])

const cmdDialog = ref(false)
const newCmd = reactive({ name: '', tag_key: '', value: 0, danger: false })

async function init() {
  try {
    const dev = await getDevice(deviceId)
    projectId = dev.data.project_id
    const cmds = await listCommands(projectId, deviceId)
    commands.value = cmds.data
  } catch {
    // 错误提示已由拦截器处理
  }
}

function addItem() { controlItems.push({ key: '', value: 0 }) }
function removeItem(index: number) {
  if (controlItems.length <= 1) { ElMessage.warning('至少保留一个控制项'); return }
  controlItems.splice(index, 1)
}
function buildTags(): Record<string, number> {
  const tags: Record<string, number> = {}
  for (const item of controlItems) {
    if (item.key.trim()) tags[item.key.trim()] = item.value
  }
  return tags
}

async function dispatch(tags: Record<string, number>, lockKey?: string) {
  stopPolling()
  lastStatus.value = null
  pendingMsg.value = ''
  sending.value = true
  if (lockKey !== undefined) sendingKey.value = lockKey
  try {
    const res = await sendDeviceControl(deviceId, tags)
    const msgId = res.data.msg_id
    pendingMsg.value = msgId
    startPolling(msgId)
  } catch {
    // 409 离线/禁用等错误已由拦截器 toast
    stopPolling()
    sending.value = false
    if (lockKey !== undefined) sendingKey.value = ''
  }
}

async function handleControl() {
  const tags = buildTags()
  if (Object.keys(tags).length === 0) { ElMessage.warning('请输入至少一个有效的标签键'); return }
  await dispatch(tags)
}

async function quickControl(btn: ControlCommand) {
  const doSend = async () => {
    await dispatch({ [btn.tag_key]: btn.value }, btn.tag_key + btn.value)
  }
  if (btn.danger) {
    try {
      await ElMessageBox.confirm(`确定要执行危险操作【${btn.name}】吗？`, '高危指令确认', {
        type: 'warning', confirmButtonText: '确认下发', cancelButtonText: '取消'
      })
    } catch {
      return
    }
  }
  await doSend()
}

function startPolling(msgId: string) {
  let tries = 0
  const maxTries = 20
  pollTimer = setInterval(async () => {
    tries++
    try {
      const res = await getControlStatus(deviceId, msgId)
      const st = res.data
      if (st.status === 'success' || st.status === 'failed' || st.status === 'timeout') {
        lastStatus.value = st
        pendingMsg.value = ''
        finishSend()
        stopPolling()
        refreshDeviceLogs() // 终态后立即刷新该设备控制日志列表
        if (st.status === 'success') ElMessage.success('设备执行成功')
        else ElMessage.error(st.ack_msg || (st.status === 'timeout' ? '设备未响应（超时）' : '设备执行失败'))
      } else {
        lastStatus.value = st
      }
    } catch {
      // 网络抖动忽略，继续轮询
    }
    if (tries >= maxTries) {
      if (!lastStatus.value || !['success', 'failed', 'timeout'].includes(lastStatus.value.status)) {
        lastStatus.value = { msg_id: msgId, status: 'timeout', ack_code: null, ack_msg: '设备未响应（超时）' }
        ElMessage.error('设备未响应（超时）')
      }
      pendingMsg.value = ''
      finishSend()
      stopPolling()
    }
  }, 1500)
}

function finishSend() {
  sending.value = false
  sendingKey.value = ''
}

function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

function statusTagType(status: string) {
  return { success: 'success', failed: 'danger', timeout: 'info', delivered: 'warning', pending: 'warning' }[status] as 'success' | 'danger' | 'info' | 'warning'
}
function statusText(st: ControlStatus) {
  return { success: '执行成功', failed: '执行失败', timeout: '超时', delivered: '已送达，等待设备', pending: '下发中' }[st.status] || st.status
}

// ===================== 该设备的控制日志 =====================
const deviceLogs = ref<ControlLog[]>([])
const logsLoading = ref(false)
let logTimer: ReturnType<typeof setInterval> | null = null

async function refreshDeviceLogs() {
  logsLoading.value = true
  try {
    const res = await getControlLogs({ device_id: deviceId, limit: 10, offset: 0 })
    deviceLogs.value = res.data.list
  } catch {
    // 错误已由拦截器提示
  } finally {
    logsLoading.value = false
  }
}

function fmtTime(v: string): string {
  const d = new Date(v)
  return isNaN(d.getTime()) ? v : d.toLocaleString('zh-CN', { hour12: false })
}

function tagsSummary(tags: Record<string, unknown> | null): string {
  if (!tags || typeof tags !== 'object') return '—'
  const entries = Object.entries(tags)
  if (!entries.length) return '—'
  return entries.map(([k, v]) => `${k}=${v}`).join(', ')
}

function logStatusTagType(status?: string) {
  return ({ success: 'success', failed: 'danger', timeout: 'info', delivered: 'warning', pending: 'warning' } as Record<string, 'success' | 'danger' | 'info' | 'warning'>)[status || ''] || 'info'
}

function logStatusText(status?: string): string {
  return ({ success: '执行成功', failed: '执行失败', timeout: '超时', delivered: '已送达，等待设备', pending: '下发中' } as Record<string, string>)[status || ''] || (status || '—')
}

function startLogAutoRefresh() {
  stopLogAutoRefresh()
  logTimer = setInterval(refreshDeviceLogs, 5000)
}
function stopLogAutoRefresh() {
  if (logTimer) { clearInterval(logTimer); logTimer = null }
}

function openCmdEditor() { cmdDialog.value = true }
async function addCommand() {
  if (!newCmd.name.trim() || !newCmd.tag_key.trim()) { ElMessage.warning('请填写按钮名称与标签键'); return }
  try {
    await createCommand({ project_id: projectId, device_id: deviceId, name: newCmd.name.trim(), tag_key: newCmd.tag_key.trim(), value: newCmd.value, danger: newCmd.danger })
    ElMessage.success('已添加')
    newCmd.name = ''; newCmd.tag_key = ''; newCmd.value = 0; newCmd.danger = false
    const cmds = await listCommands(projectId, deviceId)
    commands.value = cmds.data
  } catch {
  }
}
async function removeCommand(row: ControlCommand) {
  try {
    await ElMessageBox.confirm(`删除命令【${row.name}】？`, '确认', { type: 'warning' })
  } catch { return }
  try {
    await deleteCommand(row.id)
    commands.value = commands.value.filter(c => c.id !== row.id)
  } catch {
  }
}

onUnmounted(() => {
  stopPolling()
  stopLogAutoRefresh()
})
init()
refreshDeviceLogs()
startLogAutoRefresh()
</script>

<style scoped>
.quick-controls { display: flex; flex-wrap: wrap; }

.pending-hint {
  color: var(--wd-text-secondary);
  font-size: 13px;
}
</style>
