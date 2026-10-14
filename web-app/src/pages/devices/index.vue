<template>
  <view class="page">
    <scroll-view
      class="scroll-body"
      scroll-y
      :refresher-enabled="true"
      @refresherrefresh="onPullRefresh"
      :refresher-triggered="refreshing"
    >
      <view class="content">
        <view class="empty-state" v-if="!loading && devices.length === 0">
          <text class="empty-icon">📡</text>
          <text class="empty-text">暂无设备</text>
          <text class="empty-sub">当前项目下没有绑定设备</text>
        </view>

        <view
          class="device-card"
          v-for="item in devices"
          :key="item.id"
          @tap="goToDevice(item)"
        >
          <view class="device-status">
            <text class="status-dot" :class="statusClass(item.status)"></text>
          </view>
          <view class="device-info">
            <text class="device-name">{{ item.name }}</text>
            <text class="device-id">ID: {{ item.id }}</text>
          </view>
          <view class="device-arrow">
            <text class="arrow-icon">›</text>
          </view>
        </view>
      </view>
    </scroll-view>
  </view>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { deviceApi, type Device } from '@/api/index'
import { isLoggedIn } from '@/utils/auth'

const projectId = ref('')
const devices = ref<Device[]>([])
const loading = ref(true)
const refreshing = ref(false)

onMounted(() => {
  if (!isLoggedIn()) {
    uni.redirectTo({ url: '/pages/login/index' })
    return
  }

  const pages = getCurrentPages()
  const currentPage = pages[pages.length - 1] as any
  const options = currentPage.$page?.options || {}
  projectId.value = options.project_id || ''

  if (!projectId.value) {
    uni.showToast({ title: '缺少项目ID', icon: 'none' })
    return
  }

  fetchDevices()
})

async function fetchDevices() {
  loading.value = true
  try {
    devices.value = await deviceApi.list(projectId.value)
  } catch {
    uni.showToast({ title: '加载失败', icon: 'none' })
  } finally {
    loading.value = false
  }
}

async function onPullRefresh() {
  refreshing.value = true
  try {
    await fetchDevices()
  } finally {
    refreshing.value = false
  }
}

function statusClass(status: number): string {
  if (status === 1) return 'online'
  if (status === 2) return 'offline'
  return 'inactive'
}

function goToDevice(item: Device) {
  uni.navigateTo({
    url: `/pages/data/index?device_id=${item.id}`
  })
}
</script>

<style scoped>
.page {
  min-height: 100vh;
  background: #f5f5f5;
  display: flex;
  flex-direction: column;
}

.scroll-body {
  flex: 1;
}

.content {
  padding: 20rpx;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding-top: 200rpx;
}

.empty-icon {
  font-size: 80rpx;
}

.empty-text {
  font-size: 30rpx;
  color: #999;
  margin-top: 20rpx;
}

.empty-sub {
  font-size: 24rpx;
  color: #ccc;
  margin-top: 8rpx;
}

.device-card {
  background: #fff;
  border-radius: 16rpx;
  padding: 24rpx;
  margin-bottom: 20rpx;
  box-shadow: 0 2rpx 12rpx rgba(0, 0, 0, 0.06);
  display: flex;
  align-items: center;
}

.device-status {
  padding-right: 20rpx;
  display: flex;
  align-items: center;
}

.status-dot {
  width: 20rpx;
  height: 20rpx;
  border-radius: 50%;
  display: block;
}

.status-dot.online {
  background: #52c41a;
}

.status-dot.offline {
  background: #ff4d4f;
}

.status-dot.inactive {
  background: #d9d9d9;
}

.device-info {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.device-name {
  font-size: 30rpx;
  font-weight: 600;
  color: #1a1a1a;
}

.device-id {
  font-size: 22rpx;
  color: #bbb;
  margin-top: 6rpx;
}

.device-arrow {
  padding-left: 16rpx;
}

.arrow-icon {
  font-size: 40rpx;
  color: #ccc;
}
</style>
