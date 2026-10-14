import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  // 管理后台部署在根域，history base 必须为 '/'；
  // '/iot/' 只属于 C 端 web-app，绝不能在这里设置，否则两前端路由冲突。
  history: createWebHistory('/'),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/Login.vue'),
      meta: { requiresAuth: false }
    },
    {
      path: '/',
      component: () => import('@/components/Layout.vue'),
      meta: { requiresAuth: true },
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'Dashboard',
          component: () => import('@/views/Dashboard.vue'),
          meta: { title: '仪表盘' }
        },
        {
          path: 'tenants',
          name: 'Tenants',
          component: () => import('@/views/tenants/TenantList.vue'),
          meta: { title: '租户管理', roles: ['super_admin'] }
        },
        {
          path: 'projects',
          name: 'Projects',
          component: () => import('@/views/projects/ProjectList.vue'),
          meta: { title: '项目管理' }
        },
        {
          path: 'projects/:projectId/tags',
          name: 'ProjectTags',
          redirect: to => ({ path: `/projects/${to.params.projectId}/settings`, query: { tab: 'dictionary' } })
        },
        {
          path: 'projects/:projectId/settings',
          name: 'ProjectSettings',
          component: () => import('@/views/projects/ProjectSettings.vue'),
          meta: { title: '项目设置', back: '/projects', backLabel: '返回项目管理' }
        },
        {
          // 项目文件夹内的设备列表：复用 DeviceList（组件读路由参数锁定项目）
          path: 'projects/:projectId/devices',
          name: 'ProjectDevices',
          component: () => import('@/views/devices/DeviceList.vue'),
          meta: { title: '设备管理', back: '/devices', backLabel: '返回设备管理', projectScoped: true }
        },
        {
          // 设备管理主页：按项目卡片分类（含“全部设备”入口）
          path: 'devices',
          name: 'Devices',
          component: () => import('@/views/devices/DeviceHome.vue'),
          meta: { title: '设备管理' }
        },
        {
          // 跨项目全部设备总览
          path: 'devices/all',
          name: 'AllDevices',
          component: () => import('@/views/devices/DeviceList.vue'),
          meta: { title: '全部设备', back: '/devices', backLabel: '返回设备管理' }
        },
        {
          // 设备中心壳：页头 + Tab 页签；旧子页面路由原地保留为子路由，深链不变
          path: 'devices/:deviceId',
          name: 'DeviceDetail',
          component: () => import('@/views/devices/DeviceDetail.vue'),
          meta: { title: '设备详情', back: '/devices', backLabel: '返回设备管理' },
          children: [
            {
              path: '',
              name: 'DeviceOverview',
              component: () => import('@/views/devices/DeviceOverview.vue'),
              meta: { title: '设备详情' }
            },
            {
              path: 'tags',
              name: 'DeviceTags',
              component: () => import('@/views/devices/DeviceTags.vue'),
              meta: { title: '设备标签配置' }
            },
            {
              path: 'data',
              name: 'DeviceData',
              component: () => import('@/views/devices/DeviceData.vue'),
              meta: { title: '设备数据' }
            },
            {
              path: 'chart',
              name: 'DeviceChart',
              component: () => import('@/views/devices/DeviceChart.vue'),
              meta: { title: '数据图表' }
            },
            {
              path: 'control',
              name: 'DeviceControl',
              component: () => import('@/views/devices/DeviceControl.vue'),
              meta: { title: '设备控制' }
            },
            {
              path: 'peer',
              name: 'DevicePeerMessages',
              component: () => import('@/views/devices/DevicePeerMessages.vue'),
              meta: { title: '设备消息' }
            },
            {
              path: 'settings',
              name: 'DeviceSettings',
              component: () => import('@/views/devices/DeviceSettings.vue'),
              meta: { title: '设备设置' }
            }
          ]
        },
        {
          path: 'firmwares',
          name: 'Firmwares',
          component: () => import('@/views/firmwares/FirmwareList.vue'),
          meta: { title: '固件管理' }
        },
        {
          path: 'ota',
          name: 'OTA',
          component: () => import('@/views/ota/OTATasks.vue'),
          meta: { title: 'OTA升级' }
        },
        {
          path: 'users',
          name: 'Users',
          component: () => import('@/views/users/UserList.vue'),
          meta: { title: '用户管理', roles: ['super_admin'] }
        },
        {
          path: 'control-logs',
          name: 'ControlLogs',
          component: () => import('@/views/logs/ControlLogs.vue'),
          meta: { title: '控制日志' }
        }
      ]
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'NotFound',
      component: () => import('@/views/NotFound.vue'),
      meta: { requiresAuth: false }
    }
  ]
})

router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth !== false)

  if (requiresAuth && !authStore.token) {
    next('/login')
    return
  }

  if (to.path === '/login' && authStore.token) {
    next('/dashboard')
    return
  }

  // 角色受限路由：默认拒绝。
  // 旧逻辑 `if (to.meta.roles && role && ...)` 在 role 为空（如本地 user 损坏/丢失）
  // 时会跳过校验导致越权进入超管页面。改为：声明了 roles 的路由，必须具备有效 role 且命中，否则拒绝。
  const requiredRoles = to.meta.roles as string[] | undefined
  if (requiredRoles && requiredRoles.length > 0) {
    const role = authStore.user?.role
    if (!role || !requiredRoles.includes(role)) {
      next('/dashboard')
      return
    }
  }

  next()
})

export default router
