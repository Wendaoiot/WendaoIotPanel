<template>
  <view class="page">
    <view class="content">
      <view class="section-card">
        <view class="section-title">
          <text class="section-title-text">用户信息</text>
        </view>
        <view class="info-row">
          <text class="info-label">用户名</text>
          <text class="info-value">{{ userInfo.username || '--' }}</text>
        </view>
        <view class="info-row">
          <text class="info-label">角色</text>
          <text class="info-value">{{ roleLabel }}</text>
        </view>
        <view class="info-row" v-if="userInfo.tenant_id">
          <text class="info-label">租户ID</text>
          <text class="info-value">{{ userInfo.tenant_id }}</text>
        </view>
      </view>

      <view class="section-card">
        <view class="section-title">
          <text class="section-title-text">修改密码</text>
        </view>
        <view class="form-item">
          <text class="form-label">旧密码</text>
          <input
            class="form-input"
            v-model="oldPassword"
            type="password"
            placeholder="请输入旧密码"
            placeholder-style="color: #ccc"
          />
        </view>
        <view class="form-item">
          <text class="form-label">新密码</text>
          <input
            class="form-input"
            v-model="newPassword"
            type="password"
            placeholder="请输入新密码"
            placeholder-style="color: #ccc"
          />
        </view>
        <view class="form-item">
          <text class="form-label">确认新密码</text>
          <input
            class="form-input"
            v-model="confirmPassword"
            type="password"
            placeholder="请再次输入新密码"
            placeholder-style="color: #ccc"
          />
        </view>
        <button class="submit-btn" :disabled="changingPassword" @tap="handleChangePassword">
          {{ changingPassword ? '提交中...' : '修改密码' }}
        </button>
      </view>

      <view class="section-card">
        <button class="logout-btn" @tap="handleLogout">退出登录</button>
      </view>

      <view class="version-info">
        <text class="version-text">问稻物联 v1.0.0</text>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { userApi } from '@/api/index'
import { getUser, getUserRole, logout } from '@/utils/auth'

const userInfo = computed(() => getUser() || {})

const roleLabel = computed(() => {
  const role = getUserRole()
  if (role === 'super_admin') return '超级管理员'
  if (role === 'tenant_admin') return '租户管理员'
  return role || '--'
})

const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const changingPassword = ref(false)

async function handleChangePassword() {
  if (!oldPassword.value.trim()) {
    uni.showToast({ title: '请输入旧密码', icon: 'none' })
    return
  }
  if (!newPassword.value.trim()) {
    uni.showToast({ title: '请输入新密码', icon: 'none' })
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    uni.showToast({ title: '两次密码输入不一致', icon: 'none' })
    return
  }

  changingPassword.value = true
  try {
    await userApi.changePassword(oldPassword.value, newPassword.value)
    uni.showToast({ title: '密码修改成功', icon: 'success' })
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  } catch {
  } finally {
    changingPassword.value = false
  }
}

function handleLogout() {
  uni.showModal({
    title: '提示',
    content: '确定要退出登录吗？',
    success: (res) => {
      if (res.confirm) {
        logout()
        uni.redirectTo({ url: '/pages/login/index' })
      }
    }
  })
}
</script>

<style scoped>
.page {
  min-height: 100vh;
  background: #f5f5f5;
}

.content {
  padding: 20rpx;
}

.section-card {
  background: #fff;
  border-radius: 16rpx;
  padding: 28rpx 24rpx;
  margin-bottom: 20rpx;
  box-shadow: 0 2rpx 12rpx rgba(0, 0, 0, 0.06);
}

.section-title {
  margin-bottom: 20rpx;
  padding-bottom: 16rpx;
  border-bottom: 1rpx solid #f5f5f5;
}

.section-title-text {
  font-size: 30rpx;
  font-weight: 600;
  color: #1a1a1a;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12rpx 0;
}

.info-label {
  font-size: 26rpx;
  color: #999;
}

.info-value {
  font-size: 26rpx;
  color: #333;
}

.form-item {
  margin-bottom: 24rpx;
}

.form-label {
  font-size: 26rpx;
  color: #666;
  margin-bottom: 8rpx;
  display: block;
}

.form-input {
  width: 100%;
  height: 76rpx;
  background: #f7f8fa;
  border-radius: 12rpx;
  padding: 0 20rpx;
  font-size: 28rpx;
  box-sizing: border-box;
  border: 2rpx solid #f7f8fa;
  transition: border-color 0.2s;
}

.form-input:focus {
  border-color: #1890ff;
}

.submit-btn {
  width: 100%;
  height: 80rpx;
  line-height: 80rpx;
  background: linear-gradient(135deg, #1890ff, #096dd9);
  color: #fff;
  border: none;
  border-radius: 12rpx;
  font-size: 28rpx;
  font-weight: 600;
  margin-top: 12rpx;
}

.submit-btn[disabled] {
  opacity: 0.6;
}

.logout-btn {
  width: 100%;
  height: 80rpx;
  line-height: 80rpx;
  background: #fff;
  color: #ff4d4f;
  border: 2rpx solid #ff4d4f;
  border-radius: 12rpx;
  font-size: 28rpx;
  font-weight: 600;
}

.version-info {
  text-align: center;
  padding: 40rpx 0;
}

.version-text {
  font-size: 22rpx;
  color: #ccc;
}
</style>
