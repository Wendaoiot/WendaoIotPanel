<template>
  <view class="page">
    <view class="header-bar" v-if="projectName">
      <text class="header-title">{{ projectName }}</text>
      <text class="header-tip">实时数据监控</text>
      <view class="header-actions">
        <text class="refresh-indicator" v-if="refreshing">刷新中...</text>
        <text class="logout-link" @tap="handleLogout">退出</text>
      </view>
    </view>

    <scroll-view class="scroll-body" scroll-y :refresher-enabled="true" @refresherrefresh="onPullRefresh" :refresher-triggered="refreshing">
      <view class="content">
        <view class="empty-state" v-if="!loading && flattened.length === 0">
          <text class="empty-icon">📡</text>
          <text class="empty-text">暂无设备数据</text>
          <text class="empty-sub">请确认项目已绑定标签且设备已上报数据</text>
        </view>

        <view class="tag-card" v-for="item in flattened" :key="`${item.tag_key}-${item.device_id}`">
          <view class="card-head">
            <view class="card-title-row">
              <text class="card-tag-name">{{ item.tag_name || item.tag_key }}</text>
              <text class="card-tag-key">{{ item.tag_key }}</text>
            </view>
            <text class="card-unit" v-if="item.unit">{{ item.unit }}</text>
          </view>

          <view class="card-value-row">
            <text class="value-label">{{ item.device_name }}</text>
            <text class="value-number">{{ formatValue(item.value) }}</text>
          </view>

          <view class="card-meta">
            <text class="meta-text">{{ item.device_id }}</text>
            <text class="meta-text" v-if="item.ts">{{ formatTs(item.ts) }}</text>
          </view>

          <view class="card-control" v-if="item.writable">
            <view class="control-row">
              <input
                class="control-input"
                v-model="controlInputs[controlKey(item)]"
                :placeholder="'输入值'"
                :type="item.data_type === 'number' ? 'number' : 'text'"
                :disabled="controlPending[controlKey(item)]"
              />
              <button
                class="control-btn"
                size="mini"
                :disabled="controlPending[controlKey(item)]"
                @tap="onSendControl(item)"
              >{{ controlPending[controlKey(item)] ? '等待设备响应…' : '发送' }}</button>
            </view>
            <text
              class="control-result"
              v-if="controlResult[controlKey(item)]"
              :class="controlResult[controlKey(item)].type"
            >{{ controlResult[controlKey(item)].text }}</text>
          </view>
        </view>
      </view>
    </scroll-view>

    <view class="bottom-bar" v-if="flattened.length > 0">
      <text class="online-dot"></text>
      <text class="online-text">在线</text>
      <text class="count-text">共 {{ tagCount }} 个监测点</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, reactive } from 'vue'
import { onHide, onShow, onUnload } from '@dcloudio/uni-app'
import {
  projectApi,
  deviceApi,
  type ProjectData,
  type Project,
  type ControlStatus,
  type ControlAckStatus,
  type ControlTagValue
} from '@/api/index'
import { isLoggedIn, logout } from '@/utils/auth'

interface FlatItem {
  tag_key: string
  tag_name: string
  unit: string
  data_type: string
  writable: boolean
  device_id: string
  device_name: string
  value: number
  ts: number
  status: number
}

const projectId = ref('')
const projectName = ref('')
const loading = ref(true)
const refreshing = ref(false)
const projectData = ref<ProjectData | null>(null)
const controlInputs = reactive<Record<string, string>>({})
const controlPending = reactive<Record<string, boolean>>({})
const controlResult = reactive<Record<string, { type: 'pending' | 'success' | 'fail'; text: string }>>({})
let refreshTimer: number | null = null
// 控制结果轮询定时器：key = 控制项唯一键，value = 定时器句柄
const controlPollTimers: Record<string, ReturnType<typeof setInterval>> = {}

const flattened = computed<FlatItem[]>(() => {
  if (!projectData.value) return []
  const items: FlatItem[] = []
  for (const tag of projectData.value.tags) {
    for (const dv of tag.devices) {
      items.push({
        tag_key: tag.tag_key,
        tag_name: tag.tag_name || tag.tag_key,
        unit: tag.unit,
        data_type: tag.data_type,
        writable: tag.writable,
        device_id: dv.device_id,
        device_name: dv.device_name,
        value: dv.value,
        ts: dv.ts,
        status: dv.status
      })
    }
  }
  return items
})

const tagCount = computed(() => flattened.value.length)

onMounted(() => {
  const pages = getCurrentPages()
  const currentPage = pages[pages.length - 1] as any
  const options = currentPage.$page?.options || {}
  projectId.value = options.project_id || '1'

  if (!isLoggedIn()) {
    uni.redirectTo({ url: `/pages/login/index?project_id=${projectId.value}` })
    return
  }

  fetchProjectName()
  fetchData()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
  clearAllControlPolls()
})

// 页面隐藏/卸载时统一清理定时器，避免轮询叠加与泄漏
onHide(() => {
  stopAutoRefresh()
  cancelPendingControls()
})

onUnload(() => {
  stopAutoRefresh()
  cancelPendingControls()
})

// 回到前台时恢复 3s 自动刷新（控制轮询不恢复，需用户重新下发）
onShow(() => {
  if (projectId.value && isLoggedIn() && !refreshTimer) {
    startAutoRefresh()
  }
})

function startAutoRefresh() {
  stopAutoRefresh()
  refreshTimer = setInterval(() => {
    refreshData()
  }, 3000) as unknown as number
}

function stopAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

async function fetchProjectName() {
  try {
    const projects = await projectApi.list()
    const found = projects.find((p: Project) => String(p.id) === String(projectId.value))
    if (found) {
      projectName.value = found.name
    } else {
      projectName.value = `项目 #${projectId.value}`
    }
  } catch {
    projectName.value = `项目 #${projectId.value}`
  }
}

async function fetchData() {
  loading.value = true
  try {
    const data = await projectApi.getData(projectId.value)
    projectData.value = data
    if (!projectName.value) {
      projectName.value = `项目 #${projectId.value}`
    }
  } catch {
    uni.showToast({ title: '加载失败', icon: 'none' })
  } finally {
    loading.value = false
  }
}

async function refreshData() {
  try {
    const data = await projectApi.getData(projectId.value)
    projectData.value = data
  } catch {
  }
}

async function onPullRefresh() {
  refreshing.value = true
  try {
    await fetchData()
  } finally {
    refreshing.value = false
  }
}

function formatValue(val: number | undefined): string {
  if (val === undefined || val === null) return '--'
  if (typeof val === 'number' && !Number.isInteger(val)) {
    return val.toFixed(2)
  }
  return String(val)
}

function formatTs(ts: number): string {
  const d = new Date(ts)
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${h}:${m}:${s}`
}

async function onSendControl(item: FlatItem) {
  const key = controlKey(item)
  const raw = controlInputs[key]
  if (!raw || raw.trim() === '') {
    uni.showToast({ title: '请输入指令值', icon: 'none' })
    return
  }
  let val: ControlTagValue = raw.trim()
  if (item.data_type === 'number') {
    const num = Number(val)
    if (isNaN(num)) {
      uni.showToast({ title: '请输入有效数值', icon: 'none' })
      return
    }
    val = num
  }

  // 同一控制项正在等待响应时，忽略重复点击
  if (controlPending[key]) return
  controlPending[key] = true
  controlResult[key] = { type: 'pending', text: '等待设备响应…' }

  try {
    uni.showLoading({ title: '发送中...', mask: true })
    const res = await deviceApi.control(item.device_id, { tags: { [item.tag_key]: val } })
    uni.hideLoading()

    if (!res?.msg_id) {
      controlResult[key] = { type: 'fail', text: res?.hint || '控制指令下发失败' }
      controlPending[key] = false
      return
    }

    // 已送达网关，开始轮询设备执行结果：每 1.5s 一次，最多约 15 次（≈22.5s）
    controlInputs[key] = ''
    pollControlStatus(item.device_id, res.msg_id, key)
    await refreshData()
  } catch {
    uni.hideLoading()
    controlPending[key] = false
    controlResult[key] = { type: 'fail', text: '指令下发失败，请重试' }
  }
}

function controlKey(item: FlatItem): string {
  return `${item.tag_key}-${item.device_id}`
}

function clearControlPoll(key: string) {
  const timer = controlPollTimers[key]
  if (timer) {
    clearInterval(timer)
    delete controlPollTimers[key]
  }
}

function clearAllControlPolls() {
  Object.keys(controlPollTimers).forEach(clearControlPoll)
}

// 页面隐藏/卸载：清理所有轮询，并把仍在等待的控制项复位，避免按钮永久置灰
function cancelPendingControls() {
  clearAllControlPolls()
  Object.keys(controlPending).forEach((key) => {
    if (controlPending[key]) {
      controlPending[key] = false
      controlResult[key] = { type: 'fail', text: '已取消等待，可重新发送' }
    }
  })
}

function setControlFinish(key: string, type: 'success' | 'fail', text: string) {
  controlPending[key] = false
  controlResult[key] = { type, text }
  clearControlPoll(key)
}

function pollControlStatus(deviceId: string, msgId: string, key: string) {
  clearControlPoll(key)
  const intervalMs = 1500
  const maxAttempts = 15
  let attempts = 0

  const polling = async () => {
    attempts += 1
    try {
      const status: ControlStatus = await deviceApi.getControlStatus(deviceId, msgId)
      const st = status?.status as ControlAckStatus
      if (st === 'success') {
        setControlFinish(key, 'success', '设备执行成功')
        uni.showToast({ title: '设备执行成功', icon: 'success' })
        refreshData()
        return
      }
      if (st === 'failed' || st === 'timeout') {
        const text =
          st === 'timeout'
            ? status?.ack_msg || '设备未响应，控制超时'
            : status?.ack_msg || '设备执行失败'
        setControlFinish(key, 'fail', text)
        uni.showToast({ title: text, icon: 'none' })
        return
      }
      // pending / delivered：继续等待
    } catch {
      // 单次查询失败不中断，继续轮询直至次数用尽
    }

    if (attempts >= maxAttempts) {
      setControlFinish(key, 'fail', '设备未响应，请检查设备状态后重试')
      uni.showToast({ title: '设备未响应，请稍后重试', icon: 'none' })
    }
  }

  controlPollTimers[key] = setInterval(polling, intervalMs) as unknown as ReturnType<typeof setInterval>
}

function handleLogout() {
  logout()
  stopAutoRefresh()
  clearAllControlPolls()
  uni.redirectTo({ url: '/pages/login/index' })
}
</script>

<style scoped>
.page {
  min-height: 100vh;
  background: #f5f5f5;
  display: flex;
  flex-direction: column;
}

.header-bar {
  padding: 16rpx 28rpx 12rpx;
  background: linear-gradient(135deg, #1890ff, #096dd9);
  color: #fff;
}

.header-title {
  font-size: 34rpx;
  font-weight: 700;
  display: block;
}

.header-tip {
  font-size: 22rpx;
  opacity: 0.8;
  display: block;
  margin-top: 4rpx;
}

.header-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 16rpx;
  margin-top: -36rpx;
}

.refresh-indicator {
  font-size: 20rpx;
  opacity: 0.7;
}

.logout-link {
  font-size: 24rpx;
  opacity: 0.9;
}

.scroll-body {
  flex: 1;
}

.content {
  padding: 20rpx;
  padding-bottom: 100rpx;
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

.tag-card {
  background: #fff;
  border-radius: 16rpx;
  padding: 24rpx;
  margin-bottom: 20rpx;
  box-shadow: 0 2rpx 12rpx rgba(0, 0, 0, 0.06);
}

.card-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12rpx;
}

.card-title-row {
  display: flex;
  flex-direction: column;
  flex: 1;
}

.card-tag-name {
  font-size: 30rpx;
  font-weight: 600;
  color: #1a1a1a;
}

.card-tag-key {
  font-size: 22rpx;
  color: #aaa;
  margin-top: 4rpx;
}

.card-unit {
  font-size: 24rpx;
  color: #999;
  background: #f0f0f0;
  padding: 4rpx 12rpx;
  border-radius: 8rpx;
}

.card-value-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 8rpx;
}

.value-label {
  font-size: 24rpx;
  color: #999;
}

.value-number {
  font-size: 40rpx;
  font-weight: 700;
  color: #1890ff;
}

.card-meta {
  display: flex;
  justify-content: space-between;
  padding-top: 8rpx;
  border-top: 1rpx solid #f5f5f5;
}

.meta-text {
  font-size: 22rpx;
  color: #bbb;
}

.card-control {
  margin-top: 16rpx;
  padding-top: 16rpx;
  border-top: 1rpx dashed #eee;
}

.control-row {
  display: flex;
  align-items: center;
  gap: 12rpx;
}

.control-input {
  flex: 1;
  height: 64rpx;
  background: #f7f8fa;
  border-radius: 10rpx;
  padding: 0 16rpx;
  font-size: 26rpx;
  border: 2rpx solid #f7f8fa;
}

.control-btn {
  height: 64rpx;
  line-height: 64rpx;
  background: #1890ff;
  color: #fff;
  border: none;
  border-radius: 10rpx;
  font-size: 24rpx;
  padding: 0 24rpx;
  white-space: nowrap;
}

.control-btn[disabled] {
  background: #b0cdf0;
  color: #f0f5fb;
}

.control-result {
  display: block;
  margin-top: 10rpx;
  font-size: 22rpx;
  line-height: 1.4;
}

.control-result.pending {
  color: #faad14;
}

.control-result.success {
  color: #52c41a;
}

.control-result.fail {
  color: #ff4d4f;
}

.bottom-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 88rpx;
  background: #fff;
  border-top: 1rpx solid #eee;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8rpx;
  padding-bottom: env(safe-area-inset-bottom);
}

.online-dot {
  width: 12rpx;
  height: 12rpx;
  border-radius: 50%;
  background: #52c41a;
}

.online-text {
  font-size: 24rpx;
  color: #666;
}

.count-text {
  font-size: 22rpx;
  color: #bbb;
  margin-left: 12rpx;
}
</style>
