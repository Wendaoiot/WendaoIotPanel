<template>
  <div class="settings-page" v-loading="loading">
    <div class="ps-top">
      <div class="ps-top-left">
        <div class="ps-title-row">
          <h2 class="ps-title">{{ project?.name || '项目设置' }}</h2>
          <el-tag size="small" type="info" effect="plain">ID {{ projectId }}</el-tag>
        </div>
      </div>
      <el-tabs v-model="activeTab" class="ps-tabs" @tab-change="syncTabQuery">
        <el-tab-pane label="基本信息" name="basic" />
        <el-tab-pane label="在线判定默认" name="online" />
        <el-tab-pane label="数据字典" name="dictionary" />
        <el-tab-pane label="控制指令" name="commands" />
        <el-tab-pane label="产品（一型一密）" name="products" />
      </el-tabs>
    </div>

    <div class="ps-body">
      <!-- 基本信息 -->
      <div v-show="activeTab === 'basic'" class="tab-panel">
        <section class="panel">
          <div class="basic-split">
            <div class="basic-col basic-donut-col">
              <div class="donut-wrap">
                <svg viewBox="0 0 120 120" class="proj-donut">
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
                  <text x="60" y="55" text-anchor="middle" class="donut-total">{{ deviceCount }}</text>
                  <text x="60" y="73" text-anchor="middle" class="donut-label">设备总数</text>
                </svg>
                <ul class="donut-legend">
                  <li v-for="seg in donutSegs" :key="seg.key">
                    <i :style="{ background: seg.color }"></i>
                    <span class="dl-name">{{ seg.name }}</span>
                    <b>{{ seg.value }}</b>
                  </li>
                </ul>
              </div>
            </div>
            <div class="basic-col basic-info-col">
              <h3 class="panel-title">项目信息</h3>
              <el-form label-width="92px" class="ps-form" @submit.prevent>
                <el-form-item label="项目名称">
                  <el-input v-model="nameForm.name" maxlength="100" show-word-limit />
                </el-form-item>
                <el-form-item label="所属租户">
                  <span class="info-static">{{ tenantName }}</span>
                </el-form-item>
                <el-form-item label="创建时间">
                  <span class="info-static">{{ formatCreatedAt }}</span>
                </el-form-item>
                <el-form-item label="设备数量">
                  <span class="info-static">{{ deviceCount }} 台</span>
                </el-form-item>
                <el-form-item>
                  <el-button type="primary" :loading="savingName" :disabled="nameDirty === false" @click="saveName">保存名称</el-button>
                </el-form-item>
              </el-form>
            </div>
          </div>
        </section>
      </div>

      <!-- 在线判定默认 -->
      <div v-show="activeTab === 'online'" class="tab-panel">
        <section class="panel">
          <h3 class="panel-title">设备在线判定默认</h3>
          <el-alert type="info" :closable="false" show-icon class="ps-alert"
            title="新设备与未单独设置的存量设备按此默认判定在线状态；设备可在“设备设置”中单独修改判定方式。" />
          <el-form label-width="92px" class="ps-form">
            <el-form-item label="判定方式">
              <el-radio-group v-model="onlineForm.mode">
                <el-radio v-for="m in ONLINE_MODES" :key="m.value" :value="m.value" border>{{ m.label }}</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item v-if="onlineForm.mode === 'report' || onlineForm.mode === 'ping'" label="离线时限" class="timeout-item">
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
                  {{ onlineForm.mode === 'report'
                    ? '设备连接 broker 即在线；超过此时长未上报任何数据则判离线（设备保持连接也会判离线）。'
                    : '平台周期性发送探活，设备须回应答信号；超过此时长未收到应答则判离线。需要设备固件支持 ping 应答（常供电设备适用，休眠设备不适用）。' }}
                  全为 0 表示沿用系统时限；最长 7 天。
                </span>
              </div>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="savingOnline" :disabled="!onlineDirty" @click="saveOnline">保存默认</el-button>
              <el-button v-if="onlineDirty" @click="resetOnlineForm">放弃修改</el-button>
            </el-form-item>
          </el-form>

          <!-- 同卡片内的应用区：默认设置保存后可一键固化到存量设备 -->
          <div class="apply-block">
            <h4 class="apply-title">应用到现有设备</h4>
            <div class="danger-item">
              <div class="danger-text">
                <b>一键应用到本项目全部设备（{{ deviceCount }} 台）</b>
                <span>
                  将当前默认（{{ effectiveSummary }}）显式写入本项目每台设备，覆盖它们各自的在线判定设置；
                  此后这些设备不再动态跟随项目默认。仅在需要统一存量设备时使用。
                </span>
              </div>
              <el-button type="warning" plain @click="applyToAll">一键应用</el-button>
            </div>
          </div>
        </section>
      </div>

      <!-- 数据字典（v-if：让 el-table 挂载时按可见宽度正确分列） -->
      <div v-if="activeTab === 'dictionary'" class="tab-panel">
        <section class="panel manager-panel">
          <ProjectTagManager :project-id="projectId" />
        </section>
      </div>

      <!-- 控制指令 -->
      <div v-if="activeTab === 'commands'" class="tab-panel">
        <section class="panel manager-panel">
          <ProjectCommandManager :project-id="projectId" />
        </section>
      </div>

      <!-- 产品（一型一密）：租户级型号凭证，在项目上下文中管理 -->
      <div v-if="activeTab === 'products' && project" class="tab-panel">
        <section class="panel manager-panel">
          <ProjectProducts :tenant-id="project.tenant_id" />
        </section>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getProjects, updateProject, updateProjectSettings, applyProjectOnlineDefault,
  type Project
} from '@/api/project'
import { getTenants, type Tenant } from '@/api/tenant'
import { getDevices, type Device } from '@/api/device'
import { useAuthStore } from '@/stores/auth'
import { ONLINE_MODES, normalizeOnlineMode, onlineModeText } from '@/utils/onlineMode'
import ProjectTagManager from '@/components/ProjectTagManager.vue'
import ProjectCommandManager from '@/components/ProjectCommandManager.vue'
import ProjectProducts from '@/components/ProjectProducts.vue'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isTenantAdmin = computed(() => authStore.role === 'tenant_admin')

const projectId = computed(() => Number(route.params.projectId))
const project = ref<Project | null>(null)
const tenants = ref<Tenant[]>([])
const loading = ref(false)

// tab 与 URL query 同步（旧 tags 链接 redirect 时带 ?tab=dictionary）
const validTabs = ['basic', 'online', 'dictionary', 'commands', 'products']
const activeTab = ref((route.query.tab as string) && validTabs.includes(route.query.tab as string) ? route.query.tab as string : 'basic')
function syncTabQuery(name: string | number) {
  router.replace({ query: { ...route.query, tab: String(name) } })
}

// ---- 基本信息 ----
const nameForm = reactive({ name: '' })
const savingName = ref(false)
const nameDirty = computed(() => !!project.value && nameForm.name.trim() !== project.value.name)
const tenantName = computed(() => tenants.value.find(t => t.id === project.value?.tenant_id)?.name || `租户#${project.value?.tenant_id ?? '-'}`)
const formatCreatedAt = computed(() => {
  const raw = project.value?.created_at
  if (!raw) return '—'
  const d = new Date(raw)
  return Number.isNaN(d.getTime()) ? raw : d.toLocaleString('zh-CN', { hour12: false })
})

async function saveName() {
  if (!nameForm.name.trim()) {
    ElMessage.warning('项目名称不能为空')
    return
  }
  savingName.value = true
  try {
    await updateProject(projectId.value, { name: nameForm.name.trim() })
    ElMessage.success('已保存')
    await fetchProject()
  } catch {
    /* 拦截器已提示 */
  } finally {
    savingName.value = false
  }
}

// ---- 在线判定默认 ----
const onlineForm = reactive<{ mode: string }>({ mode: '' })
const timeout = reactive({ days: 0, hours: 0, minutes: 0, seconds: 0 })
const onlineSnapshot = ref('')
const savingOnline = ref(false)
const deviceCount = ref(0)
const projectDevices = ref<Device[]>([])

// 项目设备状态四态分布（口径与仪表盘环图一致：待激活=一型一密未激活）
const donutSegs = computed(() => {
  const list = projectDevices.value
  let online = 0, disabled = 0, pending = 0
  for (const d of list) {
    if (d.enabled === false || d.status === 2) { disabled++; continue }
    if ((d.product_id ?? 0) > 0 && !d.activated_at) { pending++; continue }
    if (d.status === 1) online++
  }
  const offline = Math.max(0, list.length - online - disabled - pending)
  const total = Math.max(1, list.length)
  const C = 2 * Math.PI * 46
  const defs = [
    { key: 'online', name: '在线', value: online, color: '#67c23a' },
    { key: 'offline', name: '离线', value: offline, color: '#c0c4cc' },
    { key: 'pending', name: '待激活', value: pending, color: '#e6a23c' },
    { key: 'disabled', name: '已禁用', value: disabled, color: '#f56c6c' }
  ]
  let acc = 0
  return defs.map(d => {
    const frac = d.value / total
    const seg = { ...d, dash: `${frac * C} ${C}`, offset: -acc * C }
    acc += frac
    return seg
  })
})

function partsToSec(): number {
  return timeout.days * 86400 + timeout.hours * 3600 + timeout.minutes * 60 + timeout.seconds
}
function secToParts(total: number) {
  return {
    days: Math.floor(total / 86400),
    hours: Math.floor((total % 86400) / 3600),
    minutes: Math.floor((total % 3600) / 60),
    seconds: total % 60
  }
}
const onlineDirty = computed(() =>
  JSON.stringify({ m: onlineForm.mode, t: timeout }) !== onlineSnapshot.value
)
const effectiveSummary = computed(() => {
  return onlineModeText(normalizeOnlineMode(onlineForm.mode), partsToSec())
})

function syncOnlineForm(p: Project | null) {
  // 项目默认固定三种显式模式；历史空值按 connection 展示
  onlineForm.mode = normalizeOnlineMode(p?.online_mode)
  Object.assign(timeout, secToParts(p?.offline_timeout_sec || 0))
  onlineSnapshot.value = JSON.stringify({ m: onlineForm.mode, t: timeout })
}
function resetOnlineForm() {
  syncOnlineForm(project.value)
}

async function saveOnline() {
  const sec = partsToSec()
  if (!onlineForm.mode) {
    ElMessage.warning('请选择判定方式')
    return
  }
  if ((onlineForm.mode === 'report' || onlineForm.mode === 'ping') && sec > 604800) {
    ElMessage.warning('时限最长为 7 天')
    return
  }
  savingOnline.value = true
  try {
    await updateProjectSettings(projectId.value, {
      online_mode: onlineForm.mode,
      offline_timeout_sec: onlineForm.mode === 'connection' ? 0 : sec
    })
    ElMessage.success('项目默认已保存')
    await fetchProject()
  } catch {
    /* 拦截器已提示 */
  } finally {
    savingOnline.value = false
  }
}

async function applyToAll() {
  const sec = partsToSec()
  try {
    await ElMessageBox.confirm(
      `将把【${effectiveSummary.value}】显式写入本项目 ${deviceCount.value} 台设备并覆盖其现有设置，确定继续？`,
      '一键应用确认',
      { type: 'warning', confirmButtonText: '应用', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    const res = await applyProjectOnlineDefault(projectId.value, {
      online_mode: onlineForm.mode,
      offline_timeout_sec: onlineForm.mode === 'connection' ? 0 : sec
    })
    ElMessage.success(`已应用到 ${res.data.affected} 台设备`)
    await fetchDeviceCount()
  } catch {
    /* 拦截器已提示 */
  }
}

// ---- 数据加载 ----
async function fetchProject() {
  loading.value = true
  try {
    const tenantParam = isTenantAdmin.value ? authStore.tenantId : undefined
    const res = await getProjects(tenantParam)
    project.value = (res.data || []).find(p => p.id === projectId.value) || null
    if (!project.value) {
      ElMessage.error('项目不存在或无权访问')
      router.replace('/projects')
      return
    }
    nameForm.name = project.value.name
    syncOnlineForm(project.value)
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function fetchTenants() {
  if (isTenantAdmin.value) return
  try {
    const res = await getTenants()
    tenants.value = res.data || []
  } catch {
    /* 仅展示用途 */
  }
}

async function fetchDeviceCount() {
  try {
    const res = await getDevices(projectId.value)
    projectDevices.value = res.data || []
    deviceCount.value = projectDevices.value.length
  } catch {
    /* 仅展示用途 */
  }
}

watch(projectId, fetchProject)
onMounted(() => {
  fetchProject()
  fetchTenants()
  fetchDeviceCount()
})
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  min-height: 100%;
}

.ps-top {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--wd-surface);
  border-bottom: 1px solid var(--wd-border);
  padding: 14px 20px 0;
}
.ps-top-left {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.ps-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ps-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--wd-text-primary);
}
.ps-tabs {
  margin-top: 6px;
}
.ps-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
}

.ps-body {
  padding: 20px;
}
.tab-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
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

/* 基本信息：同一卡片内左右两栏（编辑区 + 概况），窄屏堆叠 */
.basic-split {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 1fr);
  gap: 24px;
  align-items: stretch;
}
.basic-info-col {
  order: 1;
  min-width: 0;
  padding-right: 24px;
  border-right: 1px solid var(--wd-border-lighter);
}
.basic-info-col .ps-form :deep(.el-input) {
  max-width: 560px;
}
.basic-donut-col {
  order: 2;
  min-width: 0;
  padding: 2px 0 2px 24px;
  display: flex;
  align-self: center;
  justify-content: center;
}
.donut-wrap {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: center;
  gap: 18px;
}
.proj-donut {
  width: 108px;
  height: 108px;
  flex-shrink: 0;
}
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
.donut-legend {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: auto auto;
  gap: 6px 20px;
}
.donut-legend li {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--wd-text-regular);
  white-space: nowrap;
}
.donut-legend i { width: 9px; height: 9px; border-radius: 2px; flex-shrink: 0; }
.donut-legend b { font-size: 13px; font-variant-numeric: tabular-nums; color: var(--wd-text-primary); }
@media (max-width: 768px) {
  .basic-split {
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
  }
  .basic-info-col {
    padding-right: 0;
    border-right: none;
  }
  .basic-donut-col {
    padding: 16px 0 0;
    border-right: none;
    border-top: 1px solid var(--wd-border-lighter);
    justify-content: center;
  }
}

/* 短输入控件保持舒适宽度，不随卡片无限拉长 */
.ps-form :deep(.el-input),
.ps-form :deep(.el-select) {
  max-width: 420px;
}
.ps-form :deep(.timeout-inputs .el-input-number) {
  max-width: none;
}
/* label 不折行（“离线时限”等保持单行） */
.ps-form :deep(.el-form-item__label) {
  white-space: nowrap;
}

.ps-alert {
  margin-bottom: 16px;
}

.timeout-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.timeout-inputs {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.field-hint {
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--wd-text-secondary);
}

.info-static {
  color: var(--wd-text-regular);
  font-size: 14px;
}

.manager-panel {
  min-width: 0;
}

/* 应用区：并入在线判定卡片底部，顶部分隔 */
.apply-block {
  margin-top: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--wd-border-lighter);
}
.apply-title {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--wd-text-primary);
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
  gap: 4px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--wd-text-regular);
}
.apply-block .danger-text {
  flex: 1;
  min-width: 0;
}
@media (max-width: 600px) {
  .danger-item {
    flex-direction: column;
    align-items: stretch;
  }
  .danger-item .el-button {
    align-self: flex-end;
  }
}
</style>
