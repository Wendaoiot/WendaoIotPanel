<template>
  <div class="settings" v-loading="loading">
    <!-- 基本信息（含在线判定方式） -->
    <section class="panel">
      <h3 class="panel-title">基本信息</h3>
      <el-form :model="form" label-width="92px" class="settings-form">
        <el-form-item label="设备名称">
          <el-input v-model="form.name" maxlength="100" show-word-limit style="max-width: 320px" />
        </el-form-item>
        <el-form-item label="所属项目">
          <el-select v-model="form.projectId" style="width: 100%; max-width: 320px">
            <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="设备ID">
          <span class="mono info-static">{{ device?.id || '—' }}</span>
        </el-form-item>
        <el-form-item label="设备状态">
          <el-switch
            :model-value="device?.enabled !== false"
            inline-prompt
            active-text="启用"
            inactive-text="禁用"
            @change="(v: string | number | boolean) => toggleEnabled(Boolean(v))"
          />
          <span class="field-hint">禁用后设备无法上报数据与接收指令，并会立即断开其当前连接</span>
        </el-form-item>
        <el-form-item label="在线判定">
          <el-radio-group v-model="form.onlineMode" class="mode-radios">
            <el-radio v-for="m in ONLINE_MODES" :key="m.value" :value="m.value" border>{{ m.label }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.onlineMode === 'report' || form.onlineMode === 'ping'" label="判定离线时限">
          <div class="timeout-box">
            <div class="timeout-inputs">
              <el-input-number v-model="timeout.days" :min="0" :max="7" controls-position="right" />
              <span>天</span>
              <el-input-number v-model="timeout.hours" :min="0" :max="23" controls-position="right" />
              <span>小时</span>
              <el-input-number v-model="timeout.minutes" :min="0" :max="59" controls-position="right" />
              <span>分</span>
              <el-input-number v-model="timeout.seconds" :min="0" :max="59" :step="10" controls-position="right" />
              <span>秒</span>
            </div>
            <span class="field-hint">
              {{ form.onlineMode === 'report'
                ? '设备连接 broker 即在线；超过此时长未上报任何数据则判离线（设备保持连接也会判离线）。'
                : '平台周期性发送探活，设备须回应答信号；超过此时长未收到应答则判离线。需要设备固件支持 ping 应答（常供电设备适用，休眠设备不适用）。' }}
              全为 0 表示沿用项目默认时限；最长 7 天。
            </span>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" :disabled="!dirty" @click="save">保存</el-button>
          <el-button v-if="dirty" @click="resetForm">放弃修改</el-button>
        </el-form-item>
      </el-form>
    </section>

    <!-- 危险操作 -->
    <section class="panel danger-panel">
      <h3 class="panel-title danger-title">危险操作</h3>
      <div class="danger-actions">
        <!-- 一型一密设备：不能人工分发密钥，只能回到待激活让设备重新引导注册 -->
        <div v-if="isDynreg" class="danger-item">
          <div class="danger-text">
            <b>重新允许动态注册</b>
            <span>清空当前一机一密，设备回到待激活状态；旧密钥立即失效，设备须用产品凭证重新引导连接。</span>
          </div>
          <el-button
            type="warning"
            plain
            :disabled="isPending"
            :title="isPending ? '设备尚未激活' : ''"
            @click="handleReactivate"
          >重置激活</el-button>
        </div>
        <!-- 传统一机一密设备：人工重置接入密钥 -->
        <div v-else class="danger-item">
          <div class="danger-text">
            <b>重置接入密钥</b>
            <span>旧密钥立即失效，新密钥仅显示一次，需更新到设备端。</span>
          </div>
          <el-button type="warning" plain @click="resetSecret">重置密钥</el-button>
        </div>
        <el-divider v-if="isDynreg" />
        <div class="danger-item">
          <div class="danger-text">
            <b>删除设备</b>
            <span>将同时删除该设备的所有标签配置、数据与控制记录，不可恢复。</span>
          </div>
          <el-button type="danger" plain @click="handleDelete">删除设备</el-button>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getDevice, updateDevice, deleteDevice, setDeviceEnabled, resetDeviceSecret, reactivateDevice, type Device } from '@/api/device'
import { getProjects, type Project } from '@/api/project'
import { useAuthStore } from '@/stores/auth'
import { ONLINE_MODES, normalizeOnlineMode, type OnlineModeValue } from '@/utils/onlineMode'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isTenantAdmin = computed(() => authStore.role === 'tenant_admin')
const tenantId = computed(() => authStore.tenantId)

const deviceId = computed(() => String(route.params.deviceId || ''))
const device = ref<Device | null>(null)
const projects = ref<Project[]>([])
const loading = ref(false)
const saving = ref(false)

// 一型一密产品设备（product_id>0）
const isDynreg = computed(() => (device.value?.product_id ?? 0) > 0)
const isPending = computed(() => isDynreg.value && !device.value?.activated_at)

// 基本信息 + 在线判定统一表单
const form = reactive<{ name: string; projectId: number; onlineMode: OnlineModeValue }>({
  name: '',
  projectId: 1,
  onlineMode: 'connection'
})
const snapshot = ref('')
// 时限（天/时/分/秒）——与秒互转
const timeout = reactive({ days: 0, hours: 0, minutes: 0, seconds: 0 })
const timeoutSnapshot = ref('')

function secToParts(total: number) {
  return {
    days: Math.floor(total / 86400),
    hours: Math.floor((total % 86400) / 3600),
    minutes: Math.floor((total % 3600) / 60),
    seconds: total % 60
  }
}
function partsToSec(): number {
  return timeout.days * 86400 + timeout.hours * 3600 + timeout.minutes * 60 + timeout.seconds
}

const dirty = computed(
  () =>
    JSON.stringify({ n: form.name, p: form.projectId, m: form.onlineMode }) !== snapshot.value ||
    JSON.stringify(timeout) !== timeoutSnapshot.value
)

async function fetchDevice() {
  if (!deviceId.value) return
  loading.value = true
  try {
    const res = await getDevice(deviceId.value)
    device.value = res.data || null
    syncForms(device.value)
  } catch {
    device.value = null
  } finally {
    loading.value = false
  }
}

async function fetchProjects() {
  try {
    const res = await getProjects(isTenantAdmin.value ? tenantId.value : undefined)
    projects.value = res.data || []
  } catch {
    /* 展示用途，失败不阻塞 */
  }
}

function syncForms(d: Device | null) {
  form.name = d?.name || ''
  form.projectId = d?.project_id || 0
  form.onlineMode = normalizeOnlineMode(d?.online_mode)
  snapshot.value = JSON.stringify({ n: form.name, p: form.projectId, m: form.onlineMode })

  Object.assign(timeout, secToParts(d?.offline_timeout_sec || 0))
  timeoutSnapshot.value = JSON.stringify(timeout)
}

function resetForm() {
  form.name = device.value?.name || ''
  form.projectId = device.value?.project_id || 0
  form.onlineMode = normalizeOnlineMode(device.value?.online_mode)
  Object.assign(timeout, secToParts(device.value?.offline_timeout_sec || 0))
}

async function save() {
  if (!device.value || !form.name.trim()) {
    ElMessage.warning('设备名称不能为空')
    return
  }
  const sec = partsToSec()
  if ((form.onlineMode === 'report' || form.onlineMode === 'ping') && sec > 604800) {
    ElMessage.warning('时限最长为 7 天')
    return
  }
  const timeoutSec = form.onlineMode === 'report' || form.onlineMode === 'ping' ? sec : 0
  saving.value = true
  try {
    await updateDevice(device.value.id, {
      name: form.name.trim(),
      project_id: form.projectId,
      online_mode: form.onlineMode,
      offline_timeout_sec: timeoutSec
    })
    ElMessage.success('已保存')
    await fetchDevice()
  } catch {
    /* 拦截器已提示 */
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(enabled: boolean) {
  if (!device.value) return
  if (!enabled) {
    try {
      await ElMessageBox.confirm(
        `确定要禁用设备 ${device.value.name} (${device.value.id}) 吗？禁用后设备无法上报数据与接收指令。`,
        '确认',
        { type: 'warning' }
      )
    } catch {
      return
    }
  }
  try {
    await setDeviceEnabled(device.value.id, enabled)
    ElMessage.success(enabled ? '已启用' : '已禁用')
    fetchDevice()
  } catch {
    /* 拦截器已提示 */
  }
}

async function resetSecret() {
  if (!device.value) return
  try {
    await ElMessageBox.confirm(
      `重置设备 ${device.value.name} 的接入密钥？旧密钥将立即失效。`,
      '重置密钥',
      { type: 'warning' }
    )
  } catch {
    return
  }
  try {
    const res = await resetDeviceSecret(device.value.id)
    ElMessageBox.alert(
      `<div style="line-height:1.9;font-size:13px">`
      + `接入用户名（设备ID）：<b>${device.value.id}</b><br/>`
      + `接入密码（新密钥）：<b>${res.data.device_secret}</b><br/>`
      + `</div><br/>用户名严格区分大小写；旧密钥已失效，新密钥<b>仅此一次显示</b>，请妥善保存并更新到设备。`,
      '新接入凭证',
      { dangerouslyUseHTMLString: true, confirmButtonText: '我已保存', type: 'warning' }
    )
  } catch {
    /* 拦截器已提示 */
  }
}

async function handleReactivate() {
  if (!device.value) return
  try {
    await ElMessageBox.confirm(
      `设备 ${device.value.name} (${device.value.id}) 当前一机一密将被清空并回到待激活状态，在线连接会被断开。设备需用产品凭证重新引导注册，确定继续？`,
      '重置动态注册',
      { type: 'warning', confirmButtonText: '重置激活' }
    )
  } catch {
    return
  }
  try {
    await reactivateDevice(device.value.id)
    ElMessage.success('已回到待激活状态，等待设备重新引导注册')
    fetchDevice()
  } catch {
    /* 拦截器已提示 */
  }
}

async function handleDelete() {
  if (!device.value) return
  try {
    await ElMessageBox.confirm(
      `确定要删除设备 "${device.value.name}" (${device.value.id}) 吗？此操作将同时删除该设备的所有标签配置和数据，不可恢复。`,
      '确认删除',
      { type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger' }
    )
  } catch {
    return
  }
  try {
    await deleteDevice(device.value.id)
    ElMessage.success('设备删除成功')
    router.replace('/devices')
  } catch {
    /* 拦截器已提示 */
  }
}

onMounted(() => {
  fetchDevice()
  fetchProjects()
})
watch(deviceId, fetchDevice)
</script>

<style scoped>
.settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.panel {
  padding: 18px 20px;
  background: var(--wd-surface);
  border: 1px solid var(--wd-border);
  border-radius: var(--wd-radius-lg);
  box-shadow: var(--wd-shadow-sm);
}

.panel-title {
  margin: 0 0 14px;
  font-size: 15px;
  font-weight: 600;
  color: var(--wd-text-primary);
}

.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
}

.info-static {
  font-size: 14px;
  color: var(--wd-text-regular);
}

.settings-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.mode-radios {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.mode-radios :deep(.el-radio) {
  margin-right: 0;
}

.timeout-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.timeout-inputs {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 13px;
  color: var(--wd-text-regular);
}

.field-hint {
  font-size: 12px;
  color: var(--wd-text-secondary);
  line-height: 1.6;
}

.danger-panel {
  border-color: var(--wd-danger-border);
}

.danger-title {
  color: var(--wd-danger);
}

.danger-actions {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.danger-actions :deep(.el-divider) {
  margin: 8px 0;
}

.danger-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.danger-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 14px;
  color: var(--wd-text-primary);
}

.danger-text span {
  font-size: 12px;
  color: var(--wd-text-secondary);
}
</style>
