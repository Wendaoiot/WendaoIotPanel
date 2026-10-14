<template>
  <!-- 关闭（默认）：进入即显示全部设备；列表页脚带“项目式管理”开关 -->
  <DeviceList v-if="!prefs.device_project_view" with-view-switch />

  <!-- 开启：项目卡片二级浏览；右下角页脚提供同一开关 -->
  <ProjectDeviceGrid v-else />
</template>

<script setup lang="ts">
import { useUIPrefs } from '@/composables/useUIPrefs'
import DeviceList from './DeviceList.vue'
import ProjectDeviceGrid from '@/components/ProjectDeviceGrid.vue'

const { prefs } = useUIPrefs()
</script>
