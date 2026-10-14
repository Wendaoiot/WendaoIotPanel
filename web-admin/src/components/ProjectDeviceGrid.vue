<template>
  <div v-loading="loading" class="page-container project-grid-page">
    <div class="folder-grid-wrap">
    <div v-if="projects.length || !loading" class="folder-grid">
      <!-- 各项目 -->
      <article
        v-for="p in projects"
        :key="p.id"
        class="project-card"
        role="button"
        tabindex="0"
        :title="`进入项目「${p.name}」的设备列表`"
        @click="openProject(p)"
        @keydown.enter="openProject(p)"
      >
        <div class="pc-head">
          <span class="pc-avatar"><el-icon><FolderOpened /></el-icon></span>
          <div class="pc-title-box">
            <h3 class="pc-name" :title="p.name">{{ p.name }}</h3>
            <p v-if="!isTenantAdmin" class="pc-tenant" :title="getTenantName(p.tenant_id)">
              {{ getTenantName(p.tenant_id) }}
            </p>
          </div>
        </div>

        <div class="pc-stat-total">
          <span class="pt-num">{{ statsOf(p.id).total }}</span>
          <span class="pt-label">台设备</span>
        </div>

        <div class="pc-stats">
          <div class="ps-item">
            <span class="ps-num is-online">{{ statsOf(p.id).online }}</span>
            <span class="ps-name">在线</span>
          </div>
          <div class="ps-item">
            <span class="ps-num is-offline">{{ statsOf(p.id).offline }}</span>
            <span class="ps-name">离线</span>
          </div>
          <div class="ps-item">
            <span class="ps-num is-pending">{{ statsOf(p.id).pending }}</span>
            <span class="ps-name">待激活</span>
          </div>
          <div class="ps-item">
            <span class="ps-num is-disabled">{{ statsOf(p.id).disabled }}</span>
            <span class="ps-name">已禁用</span>
          </div>
        </div>
      </article>
    </div>
    <el-empty v-if="!projects.length && !loading" description="暂无项目，请先在「项目管理」中创建" />
    </div>

    <div class="list-footer">
      <span class="footer-count">共 {{ projects.length }} 个项目</span>
      <ProjectViewSwitch />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { FolderOpened } from '@element-plus/icons-vue'
import ProjectViewSwitch from '@/components/ProjectViewSwitch.vue'
import { getProjects, getProjectDeviceStats, type Project, type ProjectDeviceStats } from '@/api/project'
import { getTenants, type Tenant } from '@/api/tenant'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const isTenantAdmin = computed(() => authStore.role === 'tenant_admin')
const tenantId = computed(() => authStore.tenantId)
const projects = ref<Project[]>([])
const tenants = ref<Tenant[]>([])
const stats = ref<Record<string, ProjectDeviceStats>>({})
const loading = ref(false)

const emptyStats: ProjectDeviceStats = { total: 0, online: 0, offline: 0, pending: 0, disabled: 0 }
function statsOf(id: number): ProjectDeviceStats {
  return stats.value[String(id)] || emptyStats
}

function getTenantName(tid: number): string {
  const t = tenants.value.find(item => item.id === tid)
  return t?.name || `租户${tid}`
}

async function reload() {
  await Promise.all([fetchProjects(), fetchStats()])
}

async function fetchProjects() {
  loading.value = true
  try {
    const res = await getProjects(isTenantAdmin.value ? tenantId.value : undefined)
    projects.value = res.data || []
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function fetchStats() {
  try {
    const res = await getProjectDeviceStats(isTenantAdmin.value ? tenantId.value : undefined)
    stats.value = res.data || {}
  } catch {
    /* 角标仅展示，失败不阻塞 */
  }
}

async function fetchTenants() {
  try {
    const res = await getTenants()
    tenants.value = res.data || []
  } catch {
    /* 仅展示用途 */
  }
}

function openProject(p: Project) {
  router.push(`/projects/${p.id}/devices`)
}

onMounted(() => {
  if (isTenantAdmin.value) {
    reload()
  } else {
    fetchTenants()
    reload()
  }
})
</script>

<style scoped>
.project-grid-page {
  display: flex;
  flex-direction: column;
}

.folder-grid-wrap {
  flex: 1 0 auto;
  min-height: 200px;
}

.list-footer {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 16px;
  margin-top: auto;
  padding-top: 20px;
}

.footer-count {
  grid-column: 1;
  justify-self: start;
  font-size: 13px;
  color: var(--wd-text-secondary);
}

.list-footer :deep(.view-toggle) {
  grid-column: 3;
  justify-self: end;
}

.folder-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 16px;
}

.project-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px;
  background: var(--wd-surface);
  border: 1px solid var(--wd-border);
  border-radius: var(--wd-radius-lg);
  box-shadow: var(--wd-shadow-sm);
  cursor: pointer;
  outline: none;
  transition: box-shadow var(--wd-dur) var(--wd-ease), transform var(--wd-dur) var(--wd-ease), border-color var(--wd-dur) var(--wd-ease);
}

.project-card:hover,
.project-card:focus-visible {
  transform: translateY(-2px);
  border-color: var(--wd-primary-light);
  box-shadow: var(--wd-shadow-md);
}

.pc-head {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.pc-avatar {
  width: 42px;
  height: 42px;
  border-radius: var(--wd-radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex-shrink: 0;
  color: var(--wd-primary);
  background: var(--el-color-primary-light-9);
}

.pc-title-box {
  flex: 1 1 auto;
  min-width: 0;
}

.pc-name {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  line-height: 1.3;
  color: var(--wd-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pc-tenant {
  margin: 3px 0 0;
  font-size: 12px;
  color: var(--wd-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pc-stat-total {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}

.pc-stat-total .pt-num {
  font-size: 28px;
  font-weight: 700;
  line-height: 1.1;
  color: var(--wd-text-primary);
  font-variant-numeric: tabular-nums;
}

.pc-stat-total .pt-label {
  font-size: 12.5px;
  color: var(--wd-text-secondary);
}

.pc-stats {
  margin-top: auto;
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 4px;
  padding-top: 12px;
  border-top: 1px solid var(--wd-border-lighter);
}

.ps-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  min-width: 0;
}

.ps-num {
  font-size: 16px;
  font-weight: 600;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}

.ps-name {
  font-size: 11.5px;
  color: var(--wd-text-secondary);
}

.ps-num.is-online { color: var(--wd-success); }
.ps-num.is-offline { color: var(--wd-text-placeholder); }
.ps-num.is-pending { color: var(--wd-warning); }
.ps-num.is-disabled { color: var(--wd-danger); }
</style>
