<template>
  <div class="page-container device-detail">
    <!-- 单一容器：设备信息行 + 页签条组成整体吸顶栏，内容在同一容器内自然滚动，不再有叠盖 -->
    <section class="detail-shell">
      <div class="detail-top" v-loading="loading">
        <div class="detail-head">
          <div class="header-main">
            <template v-if="device">
              <div class="header-title">
                <span class="status-dot" :class="dotClass" />
                <h2 class="device-name">{{ device.name }}</h2>
                <el-tag :type="statusTagType" size="small" effect="light">{{ statusText }}</el-tag>
              </div>
              <div class="header-sub">
                <span class="mono">{{ device.id }}</span>
                <el-divider direction="vertical" />
                <span>{{ projectName }}</span>
                <el-divider direction="vertical" />
                <span>最近活跃：{{ lastActiveText }}</span>
              </div>
            </template>
            <template v-else>
              <h2 class="device-name">设备详情</h2>
            </template>
          </div>
        </div>
        <!-- Tab 页签：概览/标签/数据/图表/控制/消息/设置 -->
        <el-tabs v-model="activeTab" class="detail-tabs">
          <el-tab-pane label="概览" name="overview" />
          <el-tab-pane label="标签" name="tags" />
          <el-tab-pane label="数据" name="data" />
          <el-tab-pane label="图表" name="chart" />
          <el-tab-pane label="控制" name="control" />
          <el-tab-pane label="消息" name="peer" />
          <el-tab-pane label="设置" name="settings" />
        </el-tabs>
      </div>
      <div class="detail-body">
        <router-view :key="deviceId" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getDevice, type Device } from '@/api/device'
import { getProjects, type Project } from '@/api/project'
import { useAuthStore } from '@/stores/auth'
import { useRealtime } from '@/composables/useRealtime'
import { formatTs } from '@/utils/datetime'

const TAB_NAMES = ['overview', 'tags', 'data', 'chart', 'control', 'peer', 'settings'] as const

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const { onMessage } = useRealtime()

const deviceId = computed(() => String(route.params.deviceId || ''))
const device = ref<Device | null>(null)
const projects = ref<Project[]>([])
const loading = ref(false)

const activeTab = computed({
  get: () => {
    // /devices/:id → overview；/devices/:id/xxx → xxx
    const seg = route.path.replace(/^.*\/devices\/[^/]+\/?/, '')
    return (TAB_NAMES as readonly string[]).includes(seg) ? seg : 'overview'
  },
  set: (name: string) => {
    const target = name === 'overview' ? `/devices/${deviceId.value}` : `/devices/${deviceId.value}/${name}`
    // tab 是同一设备的视图切换：replace 不压历史，浏览器返回一次即回设备列表（深链直达仍可用）
    if (route.path !== target) router.replace(target)
  }
})

const isTenantAdmin = computed(() => authStore.role === 'tenant_admin')
const tenantId = computed(() => authStore.tenantId)

const projectName = computed(() => {
  if (!device.value) return ''
  const p = projects.value.find(item => item.id === device.value!.project_id)
  return p?.name || `项目${device.value.project_id}`
})

const statusText = computed(() => {
  if (!device.value) return ''
  if (device.value.enabled === false || device.value.status === 2) return '已禁用'
  return device.value.status === 1 ? '在线' : '离线'
})

const statusTagType = computed<'success' | 'warning' | 'danger'>(() => {
  if (!device.value) return 'warning'
  if (device.value.enabled === false || device.value.status === 2) return 'danger'
  return device.value.status === 1 ? 'success' : 'warning'
})

const dotClass = computed(() => {
  if (!device.value) return 'dot-offline'
  if (device.value.enabled === false || device.value.status === 2) return 'dot-disabled'
  return device.value.status === 1 ? 'dot-online' : 'dot-offline'
})

const lastActiveText = computed(() => (device.value?.last_active ? formatTs(device.value.last_active) : '—'))

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
    /* 忽略：项目名展示失败不影响主功能 */
  }
}

// WS 实时刷新在线状态圆点
let offRealtime: (() => void) | null = null
onMounted(() => {
  fetchDevice()
  fetchProjects()
  offRealtime = onMessage(msg => {
    if (msg.type !== 'device_status') return
    const tid = String(msg.data?.device_id ?? '')
    if (tid !== deviceId.value || !device.value) return
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

// 路由参数变化（详情页间切换）时重拉
watch(deviceId, fetchDevice)
</script>

<style scoped>
.device-detail {
}

/* 设备信息行 + 页签条组成同一条通栏吸顶栏：滚动时整条常驻，内容从不在其下方被遮挡 */
.detail-top {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--wd-surface);
  border-bottom: 1px solid var(--wd-border);
}

.detail-head {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px 20px 8px;
}

.header-main {
  flex: 1;
  min-width: 0;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.device-name {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: var(--wd-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-sub {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
  font-size: 13px;
  color: var(--wd-text-secondary);
}

.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
}

/* 状态圆点（与设备卡片同款语义色） */
.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dot-online {
  background: var(--wd-success);
  box-shadow: 0 0 0 3px var(--wd-success-bg);
}

.dot-offline {
  background: var(--wd-text-placeholder);
  box-shadow: 0 0 0 3px var(--wd-border-lighter);
}

.dot-disabled {
  background: var(--wd-danger);
  box-shadow: 0 0 0 3px var(--wd-danger-bg);
}

.detail-body {
  padding: 20px;
}

.detail-body :deep(.tab-pane) {
  padding-top: 0;
}

.detail-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 4px;
  border-bottom: none;
}

.detail-tabs :deep(.el-tabs__item) {
  font-size: 14px;
}
</style>
