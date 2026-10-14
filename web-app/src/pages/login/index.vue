<template>
  <view class="login-page">
    <view class="login-header">
      <text class="logo-text">问稻物联</text>
      <text class="sub-text">IoT SAAS Platform</text>
    </view>

    <view class="login-card">
      <view class="form-item">
        <text class="form-label">用户名</text>
        <input
          class="form-input"
          v-model="username"
          placeholder="请输入用户名"
          placeholder-style="color: #ccc"
        />
      </view>

      <view class="form-item">
        <text class="form-label">密码</text>
        <input
          class="form-input"
          v-model="password"
          type="password"
          placeholder="请输入密码"
          placeholder-style="color: #ccc"
        />
      </view>

      <view class="form-item">
        <text class="form-label">角色</text>
        <picker :range="roleOptions" @change="onRoleChange">
          <view class="picker-value">
            <text :style="{ color: selectedRole ? '#333' : '#ccc' }">
              {{ selectedRole || '请选择角色' }}
            </text>
            <text class="picker-arrow">▾</text>
          </view>
        </picker>
      </view>

      <button class="login-btn" :disabled="loading" @tap="handleLogin">
        {{ loading ? '登录中...' : '登录' }}
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { authApi, type LoginResult } from '@/api/index'
import { setToken, setUser } from '@/utils/auth'

const username = ref('')
const password = ref('')
const selectedRole = ref('')
const roleOptions = ['super_admin', 'tenant_admin']
const loading = ref(false)

function onRoleChange(e: any) {
  selectedRole.value = roleOptions[e.detail.value]
}

async function handleLogin() {
  if (!username.value.trim()) {
    uni.showToast({ title: '请输入用户名', icon: 'none' })
    return
  }
  if (!password.value.trim()) {
    uni.showToast({ title: '请输入密码', icon: 'none' })
    return
  }
  if (!selectedRole.value) {
    uni.showToast({ title: '请选择角色', icon: 'none' })
    return
  }

  loading.value = true
  uni.showLoading({ title: '登录中...', mask: true })

  try {
    const result = await authApi.login({
      username: username.value.trim(),
      password: password.value,
      role: selectedRole.value
    }) as LoginResult

    setToken(result.token)
    setUser(result.user)

    uni.hideLoading()

    uni.switchTab({
      url: '/pages/projects/index'
    })
  } catch {
    uni.hideLoading()
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  background: linear-gradient(160deg, #eaf4ff 0%, #f5f5f5 60%);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 120rpx;
}

.login-header {
  text-align: center;
  margin-bottom: 60rpx;
}

.logo-text {
  font-size: 48rpx;
  font-weight: 700;
  color: #1890ff;
  display: block;
}

.sub-text {
  font-size: 24rpx;
  color: #999;
  margin-top: 8rpx;
  display: block;
  letter-spacing: 4rpx;
}

.login-card {
  width: 640rpx;
  background: #fff;
  border-radius: 24rpx;
  padding: 48rpx 40rpx;
  box-shadow: 0 8rpx 40rpx rgba(0, 0, 0, 0.08);
}

.form-item {
  margin-bottom: 32rpx;
}

.form-label {
  font-size: 26rpx;
  color: #666;
  margin-bottom: 12rpx;
  display: block;
}

.form-input {
  width: 100%;
  height: 80rpx;
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

.picker-value {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 80rpx;
  background: #f7f8fa;
  border-radius: 12rpx;
  padding: 0 20rpx;
  font-size: 28rpx;
}

.picker-arrow {
  font-size: 28rpx;
  color: #999;
}

.login-btn {
  width: 100%;
  height: 88rpx;
  line-height: 88rpx;
  background: linear-gradient(135deg, #1890ff, #096dd9);
  color: #fff;
  border: none;
  border-radius: 12rpx;
  font-size: 30rpx;
  font-weight: 600;
  margin-top: 40rpx;
}

.login-btn[disabled] {
  opacity: 0.6;
}
</style>
