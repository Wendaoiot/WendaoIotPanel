<template>
  <div class="tab-pane">
    <div class="tab-toolbar">
      <el-button :icon="Refresh" @click="fetchMessages" :loading="loading">刷新</el-button>
    </div>

    <el-card class="table-card">
      <el-table :data="messages" v-loading="loading" stripe border>
        <el-table-column prop="id" label="ID" width="80" align="center" />
        <el-table-column prop="msg_id" label="消息ID" width="140" show-overflow-tooltip />
        <el-table-column label="方向" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.from_device_id === deviceId ? 'primary' : 'success'" size="small">
              {{ row.from_device_id === deviceId ? '发出' : '收到' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="from_device_id" label="发送方" width="160" show-overflow-tooltip />
        <el-table-column prop="to_device_id" label="接收方" width="160" show-overflow-tooltip />
        <el-table-column prop="type" label="类型" width="110" align="center" />
        <el-table-column label="Payload" min-width="260">
          <template #default="{ row }">
            <pre class="data-pre">{{ formatPayload(row.payload) }}</pre>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="180" align="center">
          <template #default="{ row }">
            {{ formatTs(row.ts) }}
          </template>
        </el-table-column>
      </el-table>
      <div v-if="messages.length === 0 && !loading" class="empty-tip">
        暂无设备间消息
      </div>
      <div v-if="total > pageSize" style="display: flex; justify-content: center; margin-top: 16px;">
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="pageSize"
          :total="total"
          layout="prev, pager, next"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Refresh } from '@element-plus/icons-vue'
import { listPeerMessages, type DevicePeerMessage } from '@/api/peer'
import { formatTs } from '@/utils/datetime'

const route = useRoute()
const deviceId = route.params.deviceId as string

const messages = ref<DevicePeerMessage[]>([])
const loading = ref(false)
const total = ref(0)
const pageSize = 50
const currentPage = ref(1)

function formatPayload(payload: string): string {
  try {
    return JSON.stringify(JSON.parse(payload), null, 2)
  } catch {
    return payload || '-'
  }
}

function statusType(status: string): 'success' | 'warning' | 'danger' | 'info' {
  if (status === 'delivered') return 'success'
  if (status === 'recipient_offline') return 'warning'
  if (status === 'rejected') return 'danger'
  return 'info'
}

function statusText(status: string): string {
  const map: Record<string, string> = {
    delivered: '已投递',
    recipient_offline: '对方离线',
    rejected: '已拒绝',
    target_not_found: '目标不存在'
  }
  return map[status] || status
}

async function fetchMessages() {
  loading.value = true
  try {
    const res = await listPeerMessages(deviceId, {
      limit: pageSize,
      offset: (currentPage.value - 1) * pageSize
    })
    messages.value = res.data.list || []
    total.value = res.data.total
  } catch {
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  currentPage.value = page
  fetchMessages()
}

onMounted(fetchMessages)
</script>

<style scoped>
.data-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 12px;
  max-height: 120px;
  overflow: auto;
}
</style>
