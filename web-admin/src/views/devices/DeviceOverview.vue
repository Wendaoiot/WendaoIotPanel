<template>
  <div class="overview" v-loading="loading">
    <section class="panel">
      <h3 class="panel-title">基本信息</h3>
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">设备ID</span>
          <span class="info-value mono">{{ device?.id || '—' }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">所属项目</span>
          <span class="info-value">{{ projectName }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">接入方式</span>
          <span class="info-value">{{ accessModeText }}</span>
        </div>
        <div v-if="isDynreg" class="info-item">
          <span class="info-label">激活时间</span>
          <span class="info-value">{{ activatedText }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">创建时间</span>
          <span class="info-value">{{ createdText }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">首次上报</span>
          <span class="info-value">{{ firstTsText }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">最近活跃</span>
          <span class="info-value">{{ lastActiveText }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">当前状态</span>
          <span class="info-value">
            <el-tag :type="statusTagType" size="small" effect="light">{{ statusText }}</el-tag>
          </span>
        </div>
        <div class="info-item">
          <span class="info-label">在线判定</span>
          <span class="info-value">
            <el-tag :type="modeTagType" size="small" effect="light">{{ modeText }}</el-tag>
          </span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { getDevice, type Device } from '@/api/device'
import { getProjects, type Project } from '@/api/project'
import { getProducts, type Product } from '@/api/product'
import { useAuthStore } from '@/stores/auth'
import { formatTs } from '@/utils/datetime'
import { resolveOnlineMode, effectiveOnlineModeText } from '@/utils/onlineMode'
import { useRealtime } from '@/composables/useRealtime'

const route = useRoute()

const authStore = useAuthStore()
const isTenantAdmin = computed(() => authStore.role === 'tenant_admin')
const tenantId = computed(() => authStore.tenantId)

const deviceId = computed(() => String(route.params.deviceId || ''))
const device = ref<Device | null>(null)
const projects = ref<Project[]>([])
const products = ref<Product[]>([])
const loading = ref(false)

const isDynreg = computed(() => (device.value?.product_id ?? 0) > 0)
const isPending = computed(() => isDynreg.value && !device.value?.activated_at)

const projectName = computed(() => {
  if (!device.value) return '—'
  const p = projects.value.find(item => item.id === device.value!.project_id)
  return p?.name || `项目${device.value.project_id}`
})

const accessModeText = computed(() => {
  if (!device.value) return '—'
  if (!isDynreg.value) return '一机一密'
  const p = products.value.find(item => item.id === device.value!.product_id)
  return p ? `一型一密 · ${p.name}` : `一型一密 · 产品${device.value.product_id}`
})

const activatedText = computed(() =>
  device.value?.activated_at ? formatTs(device.value.activated_at) : '未激活（待设备首次上线）'
)

const modeTagType = computed<'success' | 'warning' | 'info' | 'primary'>(() => {
  const mode = resolvedMode.value
  if (mode === 'report') return 'warning'
  return mode === 'ping' ? 'primary' : 'success'
})

const modeText = computed(() =>
  effectiveOnlineModeText(
    device.value?.online_mode,
    device.value?.offline_timeout_sec || 0,
    projectRecord.value?.online_mode,
    projectRecord.value?.offline_timeout_sec || 0
  )
)

const projectRecord = computed(() =>
  projects.value.find(item => item.id === device.value?.project_id) || null
)

const resolvedMode = computed(() =>
  resolveOnlineMode(device.value?.online_mode, projectRecord.value?.online_mode)
)

const statusText = computed(() => {
  if (!device.value) return '—'
  if (device.value.enabled === false || device.value.status === 2) return '已禁用'
  if (isPending.value) return '待激活'
  return device.value.status === 1 ? '在线' : '离线'
})

const statusTagType = computed<'success' | 'warning' | 'danger' | 'info'>(() => {
  if (!device.value) return 'info'
  if (device.value.enabled === false || device.value.status === 2) return 'danger'
  return device.value.status === 1 ? 'success' : 'warning'
})

const createdText = computed(() => (device.value?.created_at ? formatTs(device.value.created_at) : '—'))
const lastActiveText = computed(() => (device.value?.last_active ? formatTs(device.value.last_active) : '—'))
const firstTsText = computed(() =>
  device.value?.first_ts && device.value.first_ts > 0 ? formatTs(device.value.first_ts) : '—'
)

async function fetchDevice() {
  if (!deviceId.value) return
  loading.value = true
  try {
    const res = await getDevice(deviceId.value)
    device.value = res.data || null
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

async function fetchProducts() {
  try {
    const res = await getProducts(isTenantAdmin.value ? tenantId.value : undefined)
    products.value = res.data || []
  } catch {
    /* 展示用途，失败不阻塞 */
  }
}

// WS 实时上下线：本页“当前状态”随事件即时刷新
const { onMessage } = useRealtime()
let offRealtime: (() => void) | null = null

onMounted(() => {
  fetchDevice()
  fetchProjects()
  fetchProducts()
  offRealtime = onMessage((msg) => {
    if (String(msg.data?.device_id ?? '') !== deviceId.value || !device.value) return
    if (msg.type === 'device_activated') {
      device.value = {
        ...device.value,
        product_id: Number(msg.data?.product_id ?? device.value.product_id ?? 0),
        activated_at: msg.data?.activated_at
          ? new Date(Number(msg.data.activated_at)).toISOString()
          : device.value.activated_at
      }
      return
    }
    if (msg.type !== 'device_status') return
    if (isPending.value) return // 待激活设备无真实连接
    if (msg.data?.enabled === false) {
      device.value = { ...device.value, enabled: false, status: 2 }
    } else if (msg.data?.online) {
      device.value = { ...device.value, enabled: true, status: 1 }
    } else {
      device.value = { ...device.value, status: 0 }
    }
  })
})
onUnmounted(() => {
  offRealtime?.()
  offRealtime = null
})
watch(deviceId, fetchDevice)
</script>

<style scoped>
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

.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px 20px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.info-label {
  font-size: 12px;
  color: var(--wd-text-secondary);
}

.info-value {
  font-size: 14px;
  color: var(--wd-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
}
</style>
