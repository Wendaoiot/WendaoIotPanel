<template>
  <div class="page-container">
    <div class="toolbar">
      <div class="toolbar-left">
        <el-select
          v-if="!isTenantAdmin"
          v-model="filterTenantId"
          placeholder="按租户筛选"
          clearable
          style="width: 200px"
          @change="fetchProjects"
        >
          <el-option
            v-for="t in tenants"
            :key="t.id"
            :label="t.name"
            :value="t.id"
          />
        </el-select>
      </div>
      <div class="toolbar-right">
        <el-button type="primary" @click="showCreateDialog">
          <el-icon><Plus /></el-icon>
          新建项目
        </el-button>
      </div>
    </div>

    <el-card class="table-card">
      <el-table :data="projects" v-loading="loading" stripe border>
        <el-table-column prop="id" label="ID" width="80" align="center" />
        <el-table-column prop="name" label="项目名称" />
        <el-table-column label="所属租户" width="160" align="center">
          <template #default="{ row }">
            {{ getTenantName(row.tenant_id) }}
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180" align="center" />
        <el-table-column label="操作" width="250" align="center" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="showEditDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
            <el-button size="small" type="warning" @click="goToSettings(row)">
              <el-icon><Setting /></el-icon>&nbsp;设置
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="createDialogVisible" title="新建项目" width="450px" @closed="resetCreateForm">
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="80px">
        <el-form-item v-if="!isTenantAdmin" label="所属租户" prop="tenant_id">
          <el-select v-model="createForm.tenant_id" placeholder="请选择租户" style="width: 100%">
            <el-option
              v-for="t in tenants"
              :key="t.id"
              :label="t.name"
              :value="t.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="项目名称" prop="name">
          <el-input v-model="createForm.name" placeholder="请输入项目名称" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleCreate">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editDialogVisible" title="编辑项目" width="450px" @closed="resetEditForm">
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-width="80px">
        <el-form-item label="项目名称" prop="name">
          <el-input v-model="editForm.name" placeholder="请输入项目名称" />
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
import { ref, reactive, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Setting } from '@element-plus/icons-vue'
import { getProjects, createProject, updateProject, deleteProject, type Project } from '@/api/project'
import { getTenants, type Tenant } from '@/api/tenant'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const isTenantAdmin = computed(() => authStore.role === 'tenant_admin')
const tenantId = computed(() => authStore.tenantId)
const projects = ref<Project[]>([])
const tenants = ref<Tenant[]>([])
const loading = ref(false)
const submitting = ref(false)
const filterTenantId = ref<number | undefined>(undefined)

const createDialogVisible = ref(false)
const createFormRef = ref<FormInstance>()
const createForm = reactive({
  tenant_id: undefined as number | undefined,
  name: ''
})
const createRules: FormRules = {
  tenant_id: isTenantAdmin.value ? [] : [{ required: true, message: '请选择租户', trigger: 'change' }],
  name: [{ required: true, message: '请输入项目名称', trigger: 'blur' }]
}

const editDialogVisible = ref(false)
const editFormRef = ref<FormInstance>()
const editForm = reactive({
  id: 0,
  name: ''
})
const editRules: FormRules = {
  name: [{ required: true, message: '请输入项目名称', trigger: 'blur' }]
}

function getTenantName(tid: number): string {
  const t = tenants.value.find(item => item.id === tid)
  return t?.name || `租户${tid}`
}

async function fetchProjects() {
  loading.value = true
  try {
    const res = await getProjects(filterTenantId.value)
    projects.value = res.data || []
  } catch {
  } finally {
    loading.value = false
  }
}

async function fetchTenants() {
  try {
    const res = await getTenants()
    tenants.value = res.data || []
  } catch {
  }
}

function showCreateDialog() {
  if (isTenantAdmin.value) {
    createForm.tenant_id = tenantId.value
  }
  createDialogVisible.value = true
}

function resetCreateForm() {
  if (!isTenantAdmin.value) {
    createForm.tenant_id = undefined
  }
  createForm.name = ''
  createFormRef.value?.resetFields()
}

async function handleCreate() {
  const valid = await createFormRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    await createProject({ tenant_id: createForm.tenant_id || 0, name: createForm.name })
    ElMessage.success('项目创建成功')
    createDialogVisible.value = false
    fetchProjects()
  } catch {
  } finally {
    submitting.value = false
  }
}

function showEditDialog(row: Project) {
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
    await updateProject(editForm.id, { name: editForm.name })
    ElMessage.success('项目更新成功')
    editDialogVisible.value = false
    fetchProjects()
  } catch {
  } finally {
    submitting.value = false
  }
}

async function handleDelete(row: Project) {
  try {
    await ElMessageBox.confirm(`确定要删除项目 "${row.name}" 吗？此操作将同时删除该项目下的所有设备、标签及数据，不可恢复。`, '确认删除', {
      type: 'warning'
    })
  } catch {
    return
  }

  try {
    await deleteProject(row.id)
    ElMessage.success('项目删除成功')
    fetchProjects()
  } catch {
  }
}

function goToSettings(row: Project) {
  router.push(`/projects/${row.id}/settings`)
}

onMounted(() => {
  if (isTenantAdmin.value) {
    filterTenantId.value = tenantId.value
    fetchProjects()
  } else {
    fetchTenants()
    fetchProjects()
  }
})
</script>
