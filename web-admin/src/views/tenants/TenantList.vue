<template>
  <div class="page-container">
    <div class="toolbar">
      <div class="toolbar-right">
        <el-button type="primary" @click="showCreateDialog">
          <el-icon><Plus /></el-icon>
          新建租户
        </el-button>
      </div>
    </div>

    <el-card class="table-card">
      <el-table :data="tenants" v-loading="loading" stripe border>
        <el-table-column prop="id" label="ID" width="80" align="center" />
        <el-table-column prop="name" label="租户名称" />
        <el-table-column prop="created_at" label="创建时间" width="180" align="center" />
        <el-table-column label="操作" width="180" align="center" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="showEditDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="createDialogVisible" title="新建租户" width="450px" @closed="resetCreateForm">
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="80px">
        <el-form-item label="租户名称" prop="name">
          <el-input v-model="createForm.name" placeholder="请输入租户名称" />
        </el-form-item>
        <el-form-item label="管理员账号">
          <el-input v-model="createForm.admin_username" placeholder="可选，默认 租户名_admin" />
        </el-form-item>
        <el-form-item label="管理员密码">
          <el-input v-model="createForm.admin_pwd" type="password" placeholder="可选，默认自动生成" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleCreate">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editDialogVisible" title="编辑租户" width="450px" @closed="resetEditForm">
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-width="80px">
        <el-form-item label="租户名称" prop="name">
          <el-input v-model="editForm.name" placeholder="请输入租户名称" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleEdit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { getTenants, createTenant, updateTenant, deleteTenant, type Tenant } from '@/api/tenant'

const tenants = ref<Tenant[]>([])
const loading = ref(false)
const submitting = ref(false)

const createDialogVisible = ref(false)
const createFormRef = ref<FormInstance>()
const createForm = reactive({
  name: '',
  admin_username: '',
  admin_pwd: ''
})
const createRules: FormRules = {
  name: [{ required: true, message: '请输入租户名称', trigger: 'blur' }]
}

const editDialogVisible = ref(false)
const editFormRef = ref<FormInstance>()
const editForm = reactive({
  id: 0,
  name: ''
})
const editRules: FormRules = {
  name: [{ required: true, message: '请输入租户名称', trigger: 'blur' }]
}

async function fetchTenants() {
  loading.value = true
  try {
    const res = await getTenants()
    tenants.value = res.data || []
  } catch {
  } finally {
    loading.value = false
  }
}

function showCreateDialog() {
  createDialogVisible.value = true
}

function resetCreateForm() {
  createForm.name = ''
  createForm.admin_username = ''
  createForm.admin_pwd = ''
  createFormRef.value?.resetFields()
}

async function handleCreate() {
  const valid = await createFormRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const res = await createTenant(createForm.name, createForm.admin_pwd || undefined, createForm.admin_username || undefined)
    ElMessage.success('租户创建成功')
    createDialogVisible.value = false
    if (res.data?.admin_user || res.data?.admin_pwd) {
      ElMessageBox.alert(
        `管理员账号: ${res.data.admin_user || '-'}\n管理员密码: ${res.data.admin_pwd || '-'}\n${(res.data as unknown as Record<string, unknown>).generated ? '（密码为随机生成，仅显示一次）' : ''}`,
        '管理员凭证',
        { confirmButtonText: '我已记录', type: 'info' }
      )
    }
    fetchTenants()
  } catch {
  } finally {
    submitting.value = false
  }
}

function showEditDialog(row: Tenant) {
  editForm.id = row.id
  editForm.name = row.name
  editDialogVisible.value = true
}

function resetEditForm() {
  editForm.id = 0
  editForm.name = ''
  editFormRef.value?.resetFields()
}

async function handleEdit() {
  const valid = await editFormRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    await updateTenant(editForm.id, editForm.name)
    ElMessage.success('租户更新成功')
    editDialogVisible.value = false
    fetchTenants()
  } catch {
  } finally {
    submitting.value = false
  }
}

async function handleDelete(row: Tenant) {
  try {
    await ElMessageBox.confirm(`确定要删除租户 "${row.name}" 吗？此操作将同时删除该租户下的所有项目和设备，不可恢复。`, '确认删除', {
      type: 'warning'
    })
  } catch {
    return
  }

  try {
    await deleteTenant(row.id)
    ElMessage.success('租户删除成功')
    fetchTenants()
  } catch {
  }
}

onMounted(() => {
  fetchTenants()
})
</script>
