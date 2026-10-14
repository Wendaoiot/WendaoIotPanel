<template>
  <div class="page-container">
    <div class="toolbar">
      <div class="toolbar-right">
        <el-button type="primary" @click="showCreateDialog">
          <el-icon><Plus /></el-icon>上传固件
        </el-button>
      </div>
    </div>

    <el-card class="table-card">
      <el-table :data="list" v-loading="loading" stripe border>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="固件名称" />
        <el-table-column prop="version" label="版本" width="120" />
        <el-table-column label="大小" width="100" align="center">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column prop="md5" label="MD5" width="280" show-overflow-tooltip />
        <el-table-column prop="description" label="描述" show-overflow-tooltip />
        <el-table-column prop="created_at" label="上传时间" width="170" />
        <el-table-column label="操作" width="80" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" title="上传固件" width="500px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="固件名称" prop="name">
          <el-input v-model="form.name" placeholder="如: 温控器v2" />
        </el-form-item>
        <el-form-item label="版本号" prop="version">
          <el-input v-model="form.version" placeholder="如: 2.1.0" />
        </el-form-item>
        <el-form-item label="下载地址" prop="url">
          <el-input v-model="form.url" placeholder="固件文件的HTTP下载地址" />
        </el-form-item>
        <el-form-item label="文件大小">
          <el-input-number v-model="form.size" :min="0" placeholder="字节数" style="width: 100%" />
        </el-form-item>
        <el-form-item label="MD5">
          <el-input v-model="form.md5" placeholder="可选，固件MD5校验值" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="版本更新说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleCreate">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { getFirmwares, createFirmware, deleteFirmware } from '@/api/firmware'
import type { Firmware } from '@/api/firmware'

const list = ref<Firmware[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({ name: '', version: '', url: '', size: 0, md5: '', description: '' })

const rules: FormRules = {
  name: [{ required: true, message: '请输入固件名称', trigger: 'blur' }],
  version: [{ required: true, message: '请输入版本号', trigger: 'blur' }],
  url: [{ required: true, message: '请输入下载地址', trigger: 'blur' }]
}

function formatSize(bytes: number): string {
  if (!bytes) return '-'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1048576).toFixed(1) + ' MB'
}

async function fetchList() {
  loading.value = true
  try {
    const res = await getFirmwares()
    list.value = res.data || []
  } catch { } finally { loading.value = false }
}

function showCreateDialog() { dialogVisible.value = true }
function resetForm() { form.name = ''; form.version = ''; form.url = ''; form.size = 0; form.md5 = ''; form.description = ''; formRef.value?.resetFields() }

async function handleCreate() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    await createFirmware({ name: form.name, version: form.version, url: form.url, size: form.size, md5: form.md5, description: form.description })
    ElMessage.success('固件上传成功')
    dialogVisible.value = false
    fetchList()
  } catch { } finally { submitting.value = false }
}

async function handleDelete(row: Firmware) {
  try { await ElMessageBox.confirm(`确定删除固件 ${row.name} v${row.version}？`, '确认', { type: 'warning' }) } catch { return }
  try { await deleteFirmware(row.id); ElMessage.success('已删除'); fetchList() } catch { }
}

onMounted(() => { fetchList() })
</script>
