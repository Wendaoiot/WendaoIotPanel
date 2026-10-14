<template>
  <el-container class="layout-container">
    <el-aside width="220px" class="layout-aside">
      <div class="logo">
        <span class="logo-text">Wendao IoT</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        router
        class="layout-menu"
      >
        <el-menu-item index="/dashboard">
          <el-icon><Odometer /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>
        <el-menu-item v-if="isSuperAdmin" index="/tenants">
          <el-icon><OfficeBuilding /></el-icon>
          <span>租户管理</span>
        </el-menu-item>
        <el-menu-item v-if="isSuperAdmin" index="/users">
          <el-icon><User /></el-icon>
          <span>用户管理</span>
        </el-menu-item>
        <el-menu-item index="/projects">
          <el-icon><FolderOpened /></el-icon>
          <span>项目管理</span>
        </el-menu-item>
        <el-menu-item index="/devices">
          <el-icon><Cpu /></el-icon>
          <span>设备管理</span>
        </el-menu-item>
        <el-menu-item index="/control-logs">
          <el-icon><Document /></el-icon>
          <span>控制日志</span>
        </el-menu-item>
        <el-menu-item index="/firmwares">
          <el-icon><Box /></el-icon>
          <span>固件管理</span>
        </el-menu-item>
        <el-menu-item index="/ota">
          <el-icon><Upload /></el-icon>
          <span>OTA升级</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="layout-header">
        <div class="header-left">
          <el-tooltip v-if="backPath" :content="backLabel" placement="bottom" effect="light">
            <el-button
              circle
              class="header-back"
              @click="router.push(backPath)"
            >
              <el-icon><ArrowLeft /></el-icon>
            </el-button>
          </el-tooltip>
          <span class="header-title">{{ pageTitle }}</span>
        </div>
        <div class="header-right">
          <el-dropdown trigger="click">
            <span class="user-dropdown">
              <el-icon style="margin-right: 4px;"><UserFilled /></el-icon>
              {{ authStore.user?.username || '用户' }}
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item>
                  <span>角色: {{ authStore.user?.role === 'super_admin' ? '超级管理员' : '租户管理员' }}</span>
                </el-dropdown-item>
                <el-dropdown-item divided @click="handleLogout">
                  <span class="logout-text">退出登录</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="layout-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { useRealtime } from '@/composables/useRealtime'
import { useUIPrefs } from '@/composables/useUIPrefs'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const isSuperAdmin = computed(() => authStore.role === 'super_admin')

const { connect, disconnect } = useRealtime()
const { loadPrefs } = useUIPrefs()
onMounted(() => {
  loadPrefs(true) // 每次登录加载当前账号的服务器持久化偏好
  connect()
})
onUnmounted(disconnect)

const activeMenu = computed(() => {
  const path = route.path
  // 项目内设备列表属于“设备管理”导航范畴
  if (/^\/projects\/\d+\/devices(\/|$)/.test(path)) return '/devices'
  if (path.startsWith('/projects')) return '/projects'
  if (path.startsWith('/devices')) return '/devices'
  if (path.startsWith('/firmwares')) return '/firmwares'
  if (path.startsWith('/ota')) return '/ota'
  if (path.startsWith('/users')) return '/users'
  if (path.startsWith('/control-logs')) return '/control-logs'
  return path
})

const pageTitle = computed(() => {
  return (route.meta?.title as string) || 'Wendao IoT Platform'
})

// 详情/设置页在导航栏最左侧（标题前）提供统一返回入口（路由 meta.back 指定目标）
const backPath = computed(() => (route.meta?.back as string) || '')
const backLabel = computed(() => (route.meta?.backLabel as string) || '返回')

async function handleLogout() {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '确认退出', {
      type: 'warning'
    })
  } catch {
    return
  }
  authStore.clearAuth()
  disconnect()
  router.push('/login')
}
</script>

<style scoped>
.layout-container {
  height: 100vh;
}

.layout-aside {
  background-color: var(--wd-sidebar-bg);
  overflow-y: auto;
  overflow-x: hidden;
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--wd-surface);
  border-bottom: 1px solid var(--wd-sidebar-border);
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.3px;
}

.layout-menu {
  border-right: none;
  --el-menu-bg-color: var(--wd-sidebar-bg);
  --el-menu-text-color: var(--wd-sidebar-text);
  --el-menu-active-color: var(--wd-sidebar-active);
  --el-menu-hover-bg-color: rgba(255, 255, 255, 0.06);
}

.layout-aside .el-menu {
  border-right: none;
}

.layout-header {
  background: var(--wd-surface);
  border-bottom: 1px solid var(--wd-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 60px;
}

.header-left {
  font-size: 16px;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-title {
  color: var(--wd-text-primary);
}

.header-back {
  flex: none;
}

.logout-text {
  color: var(--wd-danger);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 14px;
}

.user-dropdown {
  display: flex;
  align-items: center;
  cursor: pointer;
  color: var(--wd-text-regular);
  font-size: 14px;
}

.layout-main {
  background: var(--wd-bg);
  min-height: calc(100vh - 60px);
  padding: 0;
}
</style>
