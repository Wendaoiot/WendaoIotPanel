<template>
  <el-tooltip
    :content="prefs.device_project_view ? '关闭后进入设备管理直接显示全部设备' : '开启后先按项目卡片分类，点卡片进入该项目设备'"
    placement="bottom"
    effect="light"
  >
    <span class="view-toggle">
      <span class="vt-label">项目式管理</span>
      <el-switch
        :model-value="prefs.device_project_view"
        :loading="saving"
        inline-prompt
        active-text="开"
        inactive-text="关"
        @change="(v: string | number | boolean) => toggle(Boolean(v))"
      />
    </span>
  </el-tooltip>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useUIPrefs } from '@/composables/useUIPrefs'

const { prefs, setProjectView } = useUIPrefs()
const saving = ref(false)

async function toggle(v: boolean) {
  saving.value = true
  try {
    await setProjectView(v)
  } catch {
    ElMessage.error('设置保存失败，请重试')
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.view-toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding-right: 4px;
}

.vt-label {
  font-size: 14px;
  color: var(--wd-text-regular);
  white-space: nowrap;
  user-select: none;
}
</style>
