<template>
  <div class="page-container device-list">
    <div class="toolbar">
      <div class="toolbar-left filter-bar">
        <el-select
          v-if="!projectScoped"
          v-model="filterProjectId"
          placeholder="按项目筛选"
          clearable
          style="width: 190px"
          @change="onFilterChanged"
        >
          <el-option
            v-for="p in projects"
            :key="p.id"
            :label="p.name"
            :value="p.id"
          />
        </el-select>
        <el-tag v-else :closable="false" type="info" effect="plain" class="scope-tag">
          <el-icon><FolderOpened /></el-icon>
          {{ scopedProjectName }}
        </el-tag>
        <el-input
          v-model="keyword"
          class="kw-input"
          placeholder="搜索 SN 或设备名称"
          clearable
          @input="scheduleSearch"
          @clear="onFilterChanged"
          @keyup.enter="onFilterChanged"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-radio-group v-model="statusFilter" size="default" @change="onFilterChanged">
          <el-radio-button value="">全部</el-radio-button>
          <el-radio-button value="online">在线</el-radio-button>
          <el-radio-button value="offline">离线</el-radio-button>
          <el-radio-button value="pending">待激活</el-radio-button>
          <el-radio-button value="disabled">已禁用</el-radio-button>
        </el-radio-group>
      </div>
      <div class="toolbar-right">
        <el-button type="primary" @click="showCreateDialog">
          <el-icon><Plus /></el-icon>
          新建设备
        </el-button>
      </div>
    </div>

    <!-- 列表视图：设备管理主页关闭“项目式管理”时，以表格列表展示全部设备 -->
    <el-card v-if="useListView" v-loading="loading" class="table-card device-table-card">
      <el-table
        :data="pageItems"
        stripe
        border
        class="device-table"
        :empty-text="emptyText"
        @row-click="goDetail"
      >
        <el-table-column label="状态" width="88">
          <template #default="{ row }">
            <span class="status-cell">
              <span class="status-dot" :class="`is-${deviceState(row)}`" />
              {{ statusText(row) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="设备名称" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cell-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="设备 ID" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono">{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="!projectScoped && !hideProjectColumn" label="所属项目" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cell-project">
              <el-icon><Folder /></el-icon>{{ getProjectName(row.project_id) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column v-if="hasProductInPage" label="产品" width="120">
          <template #default="{ row }">
            <template v-if="isDynreg(row)">{{ productName(row.product_id!) }}</template>
            <span v-else class="cell-muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="最后活跃" width="118" align="center">
          <template #default="{ row }">
            {{ isPending(row) ? '等待首次上线' : relativeActive(row.last_active) }}
          </template>
        </el-table-column>
        <el-table-column v-if="!hideModeColumn" label="在线判定" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="{ 'cell-mode': !!modeLabel(row) }">
              {{ modeLabel(row) || '仅按连接' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="72" align="center" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click.stop="goDetail(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 卡片网格：项目内设备页 / 全部设备页沿用 -->
    <div v-else v-loading="loading" class="card-grid-wrap">
      <div v-if="pageItems.length" class="device-grid">
          <article
            v-for="dev in pageItems"
            :key="dev.id"
            class="device-card"
            :class="`is-${deviceState(dev)}`"
            :style="{ '--card-glow': cardGlow(dev) }"
            role="button"
            tabindex="0"
            @click="goDetail(dev)"
            @keydown.enter="goDetail(dev)"
          >
            <span class="card-strip" :title="statusText(dev)" />
            <div class="card-inner">
              <div class="card-head">
                <span class="device-avatar"><el-icon><Cpu /></el-icon></span>
                <!-- 待激活：状态胶囊本身即橙色提示，不重复 tag；已激活产品设备在胶囊左侧放产品名 -->
                <span v-if="!isPending(dev) && isDynreg(dev)" class="mini-tag tag-product" :title="productName(dev.product_id!)">
                  {{ productName(dev.product_id!) }}
                </span>
                <span class="status-pill">{{ statusText(dev) }}</span>
              </div>
              <div class="card-body">
                <h3 class="card-name" :title="dev.name">{{ dev.name }}</h3>
                <p class="card-id mono" :title="dev.id">{{ dev.id }}</p>
              </div>
              <div class="card-meta">
                <span class="meta-item" :title="getProjectName(dev.project_id)">
                  <el-icon><Folder /></el-icon>{{ getProjectName(dev.project_id) }}
                </span>
                <span class="meta-item">
                  <el-icon><Clock /></el-icon>{{ isPending(dev) ? '等待首次上线' : relativeActive(dev.last_active) }}
                </span>
                <span v-if="modeLabel(dev)" class="meta-item meta-mode">{{ modeLabel(dev) }}</span>
              </div>
            </div>
          </article>
      </div>
      <el-empty v-else-if="!loading" :description="emptyText" />
    </div>

    <div v-if="pageTotal > 0 || withViewSwitch" class="list-footer">
      <el-pagination
        v-if="pageTotal > 0"
        v-model:current-page="page"
        :page-size="pageSize"
        :total="pageTotal"
        layout="total, prev, pager, next"
        background
        @current-change="fetchDevices"
      />
      <span v-else class="footer-count">共 0 条</span>
      <ProjectViewSwitch v-if="withViewSwitch" />
    </div>

    <el-dialog v-model="createDialogVisible" title="新建设备" width="470px" @closed="resetCreateForm">
      <el-alert
        :type="createForm.product_key ? 'warning' : 'info'"
        :closable="false"
        style="margin-bottom: 16px"
        :title="createForm.product_key
          ? '一型一密：录入后为待激活状态，设备首次以产品凭证经 mqtts://8883 引导连接时自动换取一机一密，无需人工分发密钥。'
          : '一机一密：创建后立即生成接入密钥（仅显示一次），需人工烧录到设备。'"
      />
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="84px">
        <el-form-item label="所属项目" prop="project_id">
          <el-select
            v-model="createForm.project_id"
            placeholder="请选择项目"
            style="width: 100%"
            :disabled="projectScoped"
            @change="onCreateProjectChange"
          >
            <el-option
              v-for="p in projects"
              :key="p.id"
              :label="p.name"
              :value="p.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="所属产品" prop="product_key">
          <el-select
            v-model="createForm.product_key"
            placeholder="不选 = 一机一密（传统）"
            clearable
            style="width: 100%"
          >
            <el-option
              v-for="p in filteredProducts"
              :key="p.product_key"
              :label="`${p.name}（${p.product_key}）`"
              :value="p.product_key"
            />
          </el-select>
          <div v-if="createForm.project_id && filteredProducts.length === 0" class="form-hint">
            该项目所属租户下还没有产品，可在「项目设置 → 产品」中创建；不选产品则按一机一密创建设备。
          </div>
        </el-form-item>
        <el-form-item :label="createForm.product_key ? '设备 SN' : '设备 ID'" prop="id">
          <el-input
            v-model="createForm.id"
            :placeholder="createForm.product_key
              ? '设备出厂序列号（字母/数字/_/-，≤64，区分大小写）'
              : '请输入设备唯一 ID（1-64 位字母/数字/_/-）'"
          />
        </el-form-item>
        <el-form-item label="设备名称" prop="name">
          <el-input v-model="createForm.name" :placeholder="createForm.product_key ? '选填，留空则与 SN 相同' : '请输入设备名称'" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleCreate">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Folder, FolderOpened, Clock, Search } from '@element-plus/icons-vue'
import { getDevicesPage, createDevice, preregisterDevice, type Device, type DeviceStatusFilter } from '@/api/device'
import { getProjects, type Project } from '@/api/project'
import { getProducts, type Product } from '@/api/product'
import { useAuthStore } from '@/stores/auth'
import { resolveOnlineMode, formatTimeoutDuration } from '@/utils/onlineMode'
import { useRealtime } from '@/composables/useRealtime'
import ProjectViewSwitch from '@/components/ProjectViewSwitch.vue'

// withViewSwitch：在工具栏显示“项目式管理”持久化开关（设备管理主页用）。
const props = withDefaults(defineProps<{ withViewSwitch?: boolean }>(), { withViewSwitch: false })
const withViewSwitch = computed(() => props.withViewSwitch)

// 列表视图：仅设备管理主页（带“项目式管理”开关，即已关闭项目式时）使用表格列表；
// 项目内设备页与“全部设备”页沿用卡片网格。
const useListView = computed(() => props.withViewSwitch)
// 当前页存在产品设备时才显示“产品”列，避免一机一密场景下整列空白
const hasProductInPage = computed(() => pageItems.value.some(d => isDynreg(d)))

// 窄屏响应式：隐藏次要列，避免表格横向溢出（项目列可被工具栏筛选器替代；在线判定详情页可见）
const winWidth = ref(typeof window === 'undefined' ? 1440 : window.innerWidth)
function onWinResize() { winWidth.value = window.innerWidth }
const hideProjectColumn = computed(() => winWidth.value < 1200)
const hideModeColumn = computed(() => winWidth.value < 1000)

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const isTenantAdmin = computed(() => authStore.role === 'tenant_admin')
const tenantId = computed(() => authStore.tenantId)

// 项目内页（/projects/:projectId/devices）锁定项目；全局设备页可切换
const projectScoped = computed(() => route.meta?.projectScoped === true)
const scopedProjectId = computed(() => {
  const v = Number(route.params.projectId)
  return Number.isFinite(v) && v > 0 ? v : undefined
})

const projects = ref<Project[]>([])
const products = ref<Product[]>([])

// ---- 分页检索状态 ----
const pageItems = ref<Device[]>([])
const pageTotal = ref(0)
const page = ref(1)
const pageSize = ref(24)
const keyword = ref('')
const statusFilter = ref<DeviceStatusFilter>('')
const filterProjectId = ref<number | undefined>(undefined)
const loading = ref(false)
const submitting = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

const effectiveProjectId = computed(() => (projectScoped.value ? scopedProjectId.value : filterProjectId.value))
const scopedProjectName = computed(() => {
  const p = projects.value.find(item => item.id === scopedProjectId.value)
  return p?.name || `项目 ${scopedProjectId.value ?? ''}`
})

const emptyText = computed(() => {
  if (keyword.value.trim() || statusFilter.value) return '未找到匹配的设备'
  return projectScoped.value ? '该项目暂无设备，点击右上角新建' : '暂无设备，点击右上角新建'
})

// 卡片右侧淡彩：按设备 ID 稳定哈希选色（翻页/WS 刷新不闪烁），右彩向左渐隐为白。
// 仅作极淡背景（最浓 alpha≤0.14），保证文字始终满足对比度，左侧纯白。
const cardPalette: Array<[number, number, number]> = [
  [121, 158, 255], // 蓝
  [64, 196, 196],  // 青
  [103, 194, 58],  // 绿
  [151, 118, 255], // 紫
  [230, 110, 180], // 品红
  [240, 160, 80],  // 橙
  [90, 130, 240],  // 靛
  [60, 170, 230],  // 青蓝
  [242, 120, 120], // 暖红
  [130, 200, 120]  // 草绿
]

function hashIndex(key: string): number {
  let h = 0
  for (let i = 0; i < key.length; i++) {
    h = (h * 31 + key.charCodeAt(i)) >>> 0
  }
  return h % cardPalette.length
}

function cardGlow(dev: Device): string {
  const [r, g, b] = cardPalette[hashIndex(dev.id || dev.name)]
  // to left：最右侧最浓(0.14) → 向左渐淡 → 约 68% 处完全透出白底
  return `linear-gradient(to left, rgba(${r},${g},${b},0.16) 0%, rgba(${r},${g},${b},0.06) 36%, rgba(255,255,255,0) 68%)`
}

// 一型一密：产品设备识别（product_id>0；activated_at 为空=待激活）
function isDynreg(dev: Device): boolean {
  return (dev.product_id ?? 0) > 0
}
function isPending(dev: Device): boolean {
  return isDynreg(dev) && !dev.activated_at
}
function productName(pid: number): string {
  return products.value.find(p => p.id === pid)?.name || `产品${pid}`
}

// 卡片统一状态：disabled > pending > online > offline（驱动色条/头像配色）
function deviceState(dev: Device): 'disabled' | 'pending' | 'online' | 'offline' {
  if (dev.enabled === false || dev.status === 2) return 'disabled'
  if (isPending(dev)) return 'pending'
  return dev.status === 1 ? 'online' : 'offline'
}

function getProjectName(pid: number): string {
  const p = projects.value.find(item => item.id === pid)
  return p?.name || `项目${pid}`
}

function statusText(dev: Device): string {
  if (dev.enabled === false || dev.status === 2) return '已禁用'
  if (isPending(dev)) return '待激活'
  return dev.status === 1 ? '在线' : '离线'
}

function modeLabel(dev: Device): string {
  // 生效模式 = 设备显式覆盖 -> 项目默认 -> 系统(connection)；仅按连接不在卡片重复展示
  const proj = projects.value.find(p => p.id === dev.project_id)
  const mode = resolveOnlineMode(dev.online_mode, proj?.online_mode)
  if (mode === 'connection') return ''
  const devTimeout = dev.offline_timeout_sec ?? 0
  const projTimeout = proj?.offline_timeout_sec ?? 0
  const t: number = devTimeout > 0 ? devTimeout : projTimeout
  const label = mode === 'report' ? '按上报时间' : '按应答信号'
  return t > 0 ? `${label} · 时限${formatTimeoutDuration(t)}` : label
}

function relativeActive(lastActive: string | null): string {
  if (!lastActive) return '从未活跃'
  const ms = new Date(lastActive).getTime()
  if (Number.isNaN(ms)) return '—'
  const diff = Date.now() - ms
  if (diff < 60_000) return '刚刚活跃'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  return `${Math.floor(diff / 86_400_000)} 天前`
}

async function fetchDevices() {
  loading.value = true
  try {
    const res = await getDevicesPage({
      project_id: effectiveProjectId.value,
      keyword: keyword.value,
      status: statusFilter.value,
      page: page.value,
      page_size: pageSize.value
    })
    pageItems.value = res.data?.items || []
    pageTotal.value = res.data?.total || 0
    // 请求页码超出范围（如筛选后结果变少）：回到末页
    const maxPage = Math.max(1, Math.ceil(pageTotal.value / pageSize.value))
    if (page.value > maxPage) {
      page.value = maxPage
      await fetchDevices()
    }
  } catch {
    pageItems.value = []
    pageTotal.value = 0
  } finally {
    loading.value = false
  }
}

// 任意过滤条件变化：回到第 1 页再查询
function onFilterChanged() {
  page.value = 1
  fetchDevices()
}

// 关键字防抖 300ms
function scheduleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => onFilterChanged(), 300)
}

async function fetchProjects() {
  try {
    const res = await getProjects(isTenantAdmin.value ? tenantId.value : undefined)
    projects.value = res.data || []
  } catch {
    /* 拦截器已提示 */
  }
}

async function fetchProducts() {
  try {
    const res = await getProducts(isTenantAdmin.value ? tenantId.value : undefined)
    products.value = res.data || []
  } catch {
    /* 仅用于产品名展示 */
  }
}

// ---- 新建设备（统一入口） ----
const createDialogVisible = ref(false)
const createFormRef = ref<FormInstance>()
const createForm = reactive({
  id: '',
  project_id: undefined as number | undefined,
  name: '',
  product_key: '' as string
})
const snPattern = /^[A-Za-z0-9_-]{1,64}$/
const createRules = computed<FormRules>(() => ({
  project_id: [{ required: true, message: '请选择项目', trigger: 'change' }],
  id: [
    { required: true, message: createForm.product_key ? '请输入设备 SN' : '请输入设备 ID', trigger: 'blur' },
    { pattern: snPattern, message: '1-64 位字母/数字/下划线/短横线，区分大小写', trigger: 'blur' }
  ],
  name: createForm.product_key
    ? []
    : [{ required: true, message: '请输入设备名称', trigger: 'blur' }]
}))

// 产品下拉只列所选项目同租户的产品（产品与项目须同租户）
const filteredProducts = computed<Product[]>(() => {
  if (createForm.project_id === undefined) return []
  const proj = projects.value.find(p => p.id === createForm.project_id)
  if (!proj) return []
  return products.value.filter(p => p.tenant_id === proj.tenant_id)
})

function onCreateProjectChange() {
  // 切换项目后，之前选中的产品可能不属于新项目租户
  if (!filteredProducts.value.some(p => p.product_key === createForm.product_key)) {
    createForm.product_key = ''
  }
}

function showCreateDialog() {
  createForm.project_id = effectiveProjectId.value
  createDialogVisible.value = true
}

function resetCreateForm() {
  createForm.id = ''
  createForm.project_id = projectScoped.value ? scopedProjectId.value : undefined
  createForm.name = ''
  createForm.product_key = ''
  createFormRef.value?.resetFields()
}

async function handleCreate() {
  const valid = await createFormRef.value?.validate().catch(() => false)
  if (!valid) return

  const pid = createForm.project_id!
  const sn = createForm.id.trim()
  const name = createForm.name.trim()
  submitting.value = true
  try {
    if (createForm.product_key) {
      // 一型一密：按 SN 预录为待激活，设备首连自动换一机一密，不返回人工密钥
      const res = await preregisterDevice({
        product_key: createForm.product_key,
        project_id: pid,
        sn,
        name
      })
      const row = res.data.results[0]
      if (!row?.ok) {
        ElMessage.error(row?.msg || '录入失败')
        return
      }
      createDialogVisible.value = false
      await ElMessageBox.alert(
        `设备 <b>${row.id}</b> 已录入（待激活）。<br/>设备首次使用产品凭证连接 mqtts://8883 时将自动激活，无需人工分发密钥。`,
        '录入成功',
        { dangerouslyUseHTMLString: true, confirmButtonText: '知道了', type: 'success' }
      ).catch(() => undefined)
      onFilterChanged()
      return
    }

    // 一机一密：立即生成一次性密钥，人工烧录
    const res = await createDevice({ id: sn, project_id: pid, name: name || sn })
    createDialogVisible.value = false
    const secret = res.data?.device_secret
    if (secret) {
      ElMessageBox.alert(
        `<div style="line-height:1.9;font-size:13px">`
        + `接入用户名（设备ID）：<b>${sn}</b><br/>`
        + `接入密码（密钥）：<b>${secret}</b><br/>`
        + `Broker：tcp://panel.wendaoiot.com:1883`
        + `</div><br/>用户名严格区分大小写；该密钥<b>仅此一次显示</b>，请妥善保存并配置到设备。`,
        '设备接入凭证', { dangerouslyUseHTMLString: true, confirmButtonText: '我已保存', type: 'success' }
      )
    } else {
      ElMessage.success('设备创建成功')
    }
    // 新设备可能落在其它页/排序位置，无过滤时回到第 1 页展示
    if (statusFilter.value === 'online') {
      // 新建设备离线，当前只看在线时不跳页
      fetchDevices()
    } else {
      page.value = 1
      fetchDevices()
    }
  } catch {
    /* 拦截器已提示；对话框保留便于修改 */
  } finally {
    submitting.value = false
  }
}

function goDetail(dev: Device) {
  router.push(`/devices/${dev.id}`)
}

// ---- WS 实时事件 ----
// 分页/筛选下：命中当前页的设备本地翻转；否则在“默认视图第 1 页”时节流刷新，避免与服务端排序不一致。
const { onMessage } = useRealtime()
let offRealtime: (() => void) | null = null
let refetchScheduled = false

function scheduleRefetch() {
  if (refetchScheduled) return
  refetchScheduled = true
  setTimeout(() => {
    refetchScheduled = false
    if (page.value === 1 && !keyword.value.trim() && !statusFilter.value) fetchDevices()
  }, 1500)
}

function patchDevice(id: string, patch: Partial<Device>): boolean {
  const idx = pageItems.value.findIndex(d => d.id === id)
  if (idx < 0) return false
  pageItems.value[idx] = { ...pageItems.value[idx], ...patch }
  return true
}

function initView() {
  filterProjectId.value = projectScoped.value ? scopedProjectId.value : undefined
  keyword.value = ''
  statusFilter.value = ''
  page.value = 1
  pageSize.value = 24
  fetchProjects()
  fetchProducts()
  fetchDevices()
}

// 同一 DeviceList 组件被 /projects/:id/devices 与 /devices/all 复用；路由切换（实例复用）时重新初始化。
watch(
  () => [route.name, scopedProjectId.value] as const,
  (n, o) => {
    if (!o || n[0] !== o[0] || n[1] !== o[1]) initView()
  }
)

onMounted(() => {
  initView()
  window.addEventListener('resize', onWinResize)
  offRealtime = onMessage((msg) => {
    const id = String(msg.data?.device_id ?? '')
    if (msg.type === 'device_activated') {
      const cur = pageItems.value.find(d => d.id === id)
      if (cur) {
        patchDevice(id, {
          product_id: Number(msg.data?.product_id ?? cur.product_id ?? 0),
          activated_at: msg.data?.activated_at
            ? new Date(Number(msg.data.activated_at)).toISOString()
            : cur.activated_at
        })
      } else {
        scheduleRefetch()
      }
      return
    }
    if (msg.type !== 'device_status') return
    const online = Boolean(msg.data?.online)
    const cur = pageItems.value.find(d => d.id === id)
    if (cur) {
      if (isPending(cur)) return // 待激活设备不可能有真实连接事件，忽略
      if (msg.data?.enabled === false) {
        patchDevice(id, { enabled: false, status: 2 })
      } else if (online) {
        patchDevice(id, { enabled: true, status: 1 })
      } else {
        patchDevice(id, { status: 0 })
      }
    } else {
      scheduleRefetch()
    }
  })
})

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
  offRealtime?.()
  offRealtime = null
  window.removeEventListener('resize', onWinResize)
})
</script>

<style scoped>
.device-list {
  display: flex;
  flex-direction: column;
}

.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.scope-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 32px;
  padding: 0 12px;
  max-width: 220px;
}

.scope-tag .el-icon {
  flex-shrink: 0;
}

.kw-input {
  width: 220px;
}

.card-grid-wrap {
  flex: 1 0 auto;
  min-height: 200px;
}

/* 底部页脚：三栏栅格，翻页条恒居中，项目式开关固定右下角（无开关时分页器仍居中） */
.list-footer {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 16px;
  margin-top: auto;
  padding-top: 20px;
}

.list-footer :deep(.el-pagination) {
  grid-column: 2;
  justify-self: center;
}

.list-footer .footer-count {
  grid-column: 2;
  justify-self: center;
  font-size: 13px;
  color: var(--wd-text-secondary);
}

.list-footer :deep(.view-toggle) {
  grid-column: 3;
  justify-self: end;
}

.card-grid-wrap {
  flex: 1 0 auto;
  min-height: 200px;
}

/* 自适应多列：列宽 230 左右，少卡（如仅 1 台）时不被拉成整行长条；窄屏单列 */
.device-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 250px));
  gap: 16px;
  justify-content: flex-start;
}

@media (max-width: 560px) {
  .device-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (min-width: 1800px) {
  .device-grid {
    grid-template-columns: repeat(auto-fill, minmax(240px, 260px));
  }
}

/* ===== 设备卡片：左侧状态色条 + 图标头像 + 状态胶囊，信息紧凑无空白 ===== */
.device-card {
  position: relative;
  height: 100%;
  min-height: 150px;
  background: var(--wd-surface);
  border: 1px solid var(--wd-border);
  border-radius: var(--wd-radius-lg);
  box-shadow: var(--wd-shadow-sm);
  cursor: pointer;
  overflow: hidden;
  transition: box-shadow var(--wd-dur) var(--wd-ease), transform var(--wd-dur) var(--wd-ease), border-color var(--wd-dur) var(--wd-ease);
  outline: none;
}

/* 右侧随机淡彩：最右最浓，向左渐隐为白（颜色变量由 --card-glow 注入） */
.device-card::before {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--card-glow, none);
  pointer-events: none;
  z-index: 0;
}

.device-card:hover,
.device-card:focus-visible {
  transform: translateY(-2px);
  border-color: var(--wd-primary-light);
  box-shadow: var(--wd-shadow-md);
}

.card-strip {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  background: var(--wd-text-placeholder);
  z-index: 1;
}

.is-online .card-strip { background: var(--wd-success); }
.is-disabled .card-strip { background: var(--wd-danger); }
.is-pending .card-strip { background: var(--wd-warning); }

.card-inner {
  position: relative;
  z-index: 1;
  padding: 14px 14px 12px 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
}

.card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.device-avatar {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
  color: var(--wd-text-secondary);
  background: var(--wd-border-lighter);
}

.is-online .device-avatar { color: var(--wd-success); background: var(--wd-success-bg); }
.is-disabled .device-avatar { color: var(--wd-danger); background: var(--wd-danger-bg); }
.is-pending .device-avatar { color: var(--wd-warning); background: var(--wd-warning-bg); }

.status-pill {
  margin-left: auto;
  flex-shrink: 0;
  font-size: 12px;
  line-height: 1;
  padding: 4px 9px;
  border-radius: 999px;
  color: var(--wd-text-secondary);
  background: var(--wd-border-lighter);
  white-space: nowrap;
}

.is-online .status-pill { color: var(--wd-success); background: var(--wd-success-bg); }
.is-disabled .status-pill { color: var(--wd-danger); background: var(--wd-danger-bg); }
.is-pending .status-pill { color: var(--wd-warning); background: var(--wd-warning-bg); }

.mini-tag {
  flex-shrink: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1;
  padding: 4px 8px;
  border-radius: 6px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tag-pending {
  color: var(--wd-warning);
  background: var(--wd-warning-bg);
  font-weight: 600;
}

.tag-product {
  color: var(--wd-success);
  background: var(--wd-success-bg);
}

.card-body {
  min-width: 0;
}

.card-name {
  margin: 0 0 3px;
  font-size: 15px;
  font-weight: 600;
  color: var(--wd-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-id {
  margin: 0;
  font-size: 12px;
  color: var(--wd-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
}

.card-meta {
  margin-top: auto;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  padding-top: 10px;
  border-top: 1px solid var(--wd-border-lighter);
}

.meta-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 100%;
  min-width: 0;
  font-size: 12px;
  color: var(--wd-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta-item .el-icon {
  flex-shrink: 0;
}

.meta-mode {
  color: var(--wd-primary);
}

.form-hint {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--wd-text-secondary);
}

.form-hint a {
  color: var(--wd-primary);
}

/* ===== 列表视图（关闭项目式管理时） ===== */
.device-table-card {
  margin-top: 0;
}

.device-table-card :deep(.el-card__body) {
  padding: 12px;
}

/* 整行可点击进入详情 */
.device-table :deep(.el-table__row) {
  cursor: pointer;
}

.status-cell {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  color: var(--wd-text-regular);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--wd-text-placeholder);
}

.status-dot.is-online { background: var(--wd-success); }
.status-dot.is-offline { background: var(--wd-text-placeholder); }
.status-dot.is-pending { background: var(--wd-warning); }
.status-dot.is-disabled { background: var(--wd-danger); }

.cell-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--wd-text-primary);
}

.cell-project {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 13px;
  color: var(--wd-text-secondary);
}

.cell-project .el-icon {
  flex-shrink: 0;
}

.cell-muted {
  color: var(--wd-text-placeholder);
}

.cell-mode {
  color: var(--wd-primary);
}
</style>
