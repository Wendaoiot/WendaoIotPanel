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
        <view class="empty-state" v-if="!loading && projects.length === 0">
          <text class="empty-icon">📂</text>
          <text class="empty-text">暂无项目</text>
          <text class="empty-sub">请联系管理员分配项目</text>
        </view>

        <view
          class="project-card"
          v-for="item in projects"
          :key="item.id"
          @tap="goToProject(item)"
        >
          <view class="card-body">
            <text class="card-name">{{ item.name }}</text>
            <text class="card-desc" v-if="item.description">{{ item.description }}</text>
          </view>
          <view class="card-arrow">
            <text class="arrow-icon">›</text>
          </view>
        </view>
      </view>
    </scroll-view>
  </view>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { projectApi, type Project } from '@/api/index'
import { isLoggedIn } from '@/utils/auth'

const projects = ref<Project[]>([])
const loading = ref(true)
const refreshing = ref(false)

onMounted(() => {
  if (!isLoggedIn()) {
    uni.redirectTo({ url: '/pages/login/index' })
    return
  }
  fetchProjects()
})

async function fetchProjects() {
  loading.value = true
  try {
    projects.value = await projectApi.list()
  } catch {
    uni.showToast({ title: '加载失败', icon: 'none' })
  } finally {
    loading.value = false
  }
}

async function onPullRefresh() {
  refreshing.value = true
  try {
    await fetchProjects()
  } finally {
    refreshing.value = false
  }
}

function goToProject(item: Project) {
  uni.navigateTo({
    url: `/pages/index/index?project_id=${item.id}`
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

.project-card {
  background: #fff;
  border-radius: 16rpx;
  padding: 28rpx 24rpx;
  margin-bottom: 20rpx;
  box-shadow: 0 2rpx 12rpx rgba(0, 0, 0, 0.06);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-body {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.card-name {
  font-size: 32rpx;
  font-weight: 600;
  color: #1a1a1a;
}

.card-desc {
  font-size: 24rpx;
  color: #999;
  margin-top: 8rpx;
}

.card-arrow {
  padding-left: 16rpx;
}

.arrow-icon {
  font-size: 40rpx;
  color: #ccc;
}
</style>
