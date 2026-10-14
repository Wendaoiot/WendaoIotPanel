<template>
  <div class="not-found-container">
    <div class="not-found-card">
      <h1 style="font-size: 72px; color: #909399; margin-bottom: 0;">404</h1>
      <p style="font-size: 16px; color: #606266; margin: 16px 0;">页面不存在</p>
      <el-button type="primary" @click="$router.push('/dashboard')">返回首页</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
</script>

<style scoped>
.not-found-container {
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background: #f0f2f5;
}

.not-found-card {
  text-align: center;
  padding: 60px;
}
</style>
