import { ref } from 'vue'
import { getPreferences, updatePreferences, type UIPreferences } from '@/api/prefs'

// 模块级单例：整个会话只加载一次，所有页面共享同一份偏好
const prefs = ref<UIPreferences>({ device_project_view: false })
const loaded = ref(false)
let loadingPromise: Promise<void> | null = null

async function loadPrefs(force = false): Promise<void> {
  if (loaded.value && !force) return
  if (loadingPromise && !force) return loadingPromise
  loadingPromise = (async () => {
    try {
      const res = await getPreferences()
      prefs.value = res.data ?? { device_project_view: false }
      loaded.value = true
    } catch {
      // 偏好加载失败不阻塞使用，回落默认（全部设备视图）
      prefs.value = { device_project_view: false }
    } finally {
      loadingPromise = null
    }
  })()
  return loadingPromise
}

async function setProjectView(on: boolean): Promise<void> {
  // 非乐观更新：等服务器确认后再改响应式状态。
  // 否则同步改值会立即触发设备主页 v-if 卸载当前 el-switch，EP change 处理器访问已移除 input 报 null。
  const res = await updatePreferences({ ...prefs.value, device_project_view: on })
  prefs.value = res.data ?? { device_project_view: on }
  loaded.value = true
}

export function useUIPrefs() {
  return { prefs, loaded, loadPrefs, setProjectView }
}
