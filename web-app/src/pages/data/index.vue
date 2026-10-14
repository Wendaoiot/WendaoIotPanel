<template>
  <view class="page">
    <view class="toolbar">
      <view class="auto-refresh-row" @tap="toggleAutoRefresh">
        <text class="toggle-label">自动刷新</text>
        <view class="toggle-switch" :class="{ active: autoRefresh }">
          <view class="toggle-knob"></view>
        </view>
      </view>
    </view>

    <scroll-view
      class="scroll-body"
      scroll-y
      @scrolltolower="loadMore"
      :refresher-enabled="true"
      @refresherrefresh="onPullRefresh"
      :refresher-triggered="refreshing"
    >
      <view class="content">
        <view class="empty-state" v-if="!loading && dataList.length === 0">
          <text class="empty-icon">📊</text>
          <text class="empty-text">暂无数据记录</text>
        </view>

        <view class="data-table" v-if="dataList.length > 0">
          <view class="table-header">
            <text class="col-time">时间</text>
            <text class="col-data">数据</text>
          </view>

          <view class="table-row" v-for="(item, idx) in dataList" :key="idx">
            <text class="col-time cell-time">{{ formatTime(item.ts) }}</text>
            <text class="col-data cell-data">{{ formatData(item.data) }}</text>
          </view>

          <view class="load-more" v-if="hasMore">
            <text class="load-more-text">{{ loadingMore ? '加载中...' : '上拉加载更多' }}</text>
          </view>
          <view class="load-more" v-else-if="dataList.length > 0">
            <text class="load-more-text">没有更多数据</text>
          </view>
        </view>
      </view>
    </scroll-view>
  </view>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { deviceApi, type DeviceDataItem } from '@/api/index'
import { isLoggedIn } from '@/utils/auth'

const deviceId = ref('')
const dataList = ref<DeviceDataItem[]>([])
const total = ref(0)
const offset = ref(0)
const limit = 20
const loading = ref(true)
const loadingMore = ref(false)
const refreshing = ref(false)
const autoRefresh = ref(false)
let autoRefreshTimer: number | null = null

const hasMore = ref(true)

onMounted(() => {
  if (!isLoggedIn()) {
    uni.redirectTo({ url: '/pages/login/index' })
    return
  }

  const pages = getCurrentPages()
  const currentPage = pages[pages.length - 1] as any
  const options = currentPage.$page?.options || {}
  deviceId.value = options.device_id || ''

  if (!deviceId.value) {
    uni.showToast({ title: '缺少设备ID', icon: 'none' })
    return
  }

  fetchInitial()
})

onUnmounted(() => {
  stopAutoRefresh()
})

async function fetchInitial() {
  loading.value = true
  offset.value = 0
  dataList.value = []
  hasMore.value = true
  try {
    const result = await deviceApi.getData(deviceId.value, { limit, offset: 0 })
    dataList.value = result.list || []
    total.value = result.total || 0
    offset.value = dataList.value.length
    hasMore.value = dataList.value.length < total.value
  } catch {
    uni.showToast({ title: '加载失败', icon: 'none' })
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  try {
    const result = await deviceApi.getData(deviceId.value, { limit, offset: offset.value })
    const list = result.list || []
    dataList.value = [...dataList.value, ...list]
    total.value = result.total || 0
    offset.value = dataList.value.length
    hasMore.value = dataList.value.length < total.value
  } catch {
    uni.showToast({ title: '加载失败', icon: 'none' })
  } finally {
    loadingMore.value = false
  }
}

async function onPullRefresh() {
  refreshing.value = true
  try {
    await fetchInitial()
  } finally {
    refreshing.value = false
  }
}

function toggleAutoRefresh() {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    startAutoRefresh()
  } else {
    stopAutoRefresh()
  }
}

function startAutoRefresh() {
  stopAutoRefresh()
  autoRefreshTimer = setInterval(() => {
    fetchInitial()
  }, 5000) as unknown as number
}

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

function formatTime(ts: number): string {
  if (!ts) return '--'
  const d = new Date(ts)
  const y = d.getFullYear()
  const mo = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${mo}-${day} ${h}:${m}:${s}`
}

function formatData(data: Record<string, any>): string {
  if (!data) return '--'
  const entries = Object.entries(data)
  if (entries.length === 0) return '--'
  return entries.map(([k, v]) => `${k}: ${v}`).join('  ')
}
</script>

<style scoped>
.page {
  min-height: 100vh;
  background: #f5f5f5;
  display: flex;
  flex-direction: column;
}

.toolbar {
  padding: 16rpx 28rpx;
  background: #fff;
  border-bottom: 1rpx solid #eee;
}

.auto-refresh-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12rpx;
}

.toggle-label {
  font-size: 26rpx;
  color: #666;
}

.toggle-switch {
  width: 80rpx;
  height: 44rpx;
  border-radius: 22rpx;
  background: #ddd;
  position: relative;
  transition: background 0.3s;
}

.toggle-switch.active {
  background: #1890ff;
}

.toggle-knob {
  width: 36rpx;
  height: 36rpx;
  border-radius: 50%;
  background: #fff;
  position: absolute;
  top: 4rpx;
  left: 4rpx;
  transition: left 0.3s;
}

.toggle-switch.active .toggle-knob {
  left: 40rpx;
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

.data-table {
  background: #fff;
  border-radius: 16rpx;
  box-shadow: 0 2rpx 12rpx rgba(0, 0, 0, 0.06);
  overflow: hidden;
}

.table-header {
  display: flex;
  padding: 20rpx 24rpx;
  background: #fafafa;
  border-bottom: 1rpx solid #eee;
}

.table-row {
  display: flex;
  padding: 20rpx 24rpx;
  border-bottom: 1rpx solid #f5f5f5;
}

.table-row:last-child {
  border-bottom: none;
}

.col-time {
  width: 300rpx;
  font-size: 24rpx;
  color: #999;
}

.col-data {
  flex: 1;
  font-size: 24rpx;
  color: #333;
}

.cell-time {
  font-size: 22rpx;
  color: #999;
}

.cell-data {
  font-size: 22rpx;
  color: #333;
  word-break: break-all;
}

.load-more {
  padding: 20rpx;
  text-align: center;
}

.load-more-text {
  font-size: 24rpx;
  color: #bbb;
}
</style>
