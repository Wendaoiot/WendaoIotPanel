<template>
  <div class="tag-manager">
    <div class="tm-toolbar">
      <span class="tm-desc">声明本项目设备通用的数据点（字段名、类型、方向），用于项目数据聚合与展示。</span>
      <el-button type="primary" @click="dialogVisible = true">
        <el-icon><Plus /></el-icon>
        新建数据点
      </el-button>
    </div>

    <el-table ref="tableRef" :data="tags" v-loading="loading" stripe border size="default">
      <el-table-column prop="id" label="ID" width="72" align="center" />
      <el-table-column prop="tag_key" label="标签键" min-width="150" show-overflow-tooltip />
      <el-table-column prop="tag_name" label="标签名称" min-width="150" show-overflow-tooltip />
      <el-table-column label="单位" min-width="90" align="center">
        <template #default="{ row }">{{ row.unit || '—' }}</template>
      </el-table-column>
      <el-table-column label="数据类型" min-width="100" align="center">
        <template #default="{ row }">
          <el-tag size="small">{{ dataTypeLabel(row.data_type) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column min-width="118" align="center">
        <template #header>
          <span class="col-head">
            方向
            <el-tooltip placement="top" effect="light">
              <template #content>
                <span class="dir-tip">仅上报：设备到平台的遥测值（如温度）；<br />可下行设置：平台可反向设置的量（如开关、阈值），实际下发在“设备控制”中配置。</span>
              </template>
              <el-icon class="col-info"><InfoFilled /></el-icon>
            </el-tooltip>
          </span>
        </template>
        <template #default="{ row }">
          <el-tag :type="row.writable ? 'warning' : 'success'" size="small">{{ row.writable ? '可下行设置' : '仅上报' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="104" align="center" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="danger" @click="handleRemove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="新建数据点" width="500px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="92px">
        <el-form-item label="标签键" prop="tag_key">
          <el-input v-model="form.tag_key" placeholder="须与设备上报字段一致，如 temp、relay1" />
        </el-form-item>
        <el-form-item label="标签名称" prop="tag_name">
          <el-input v-model="form.tag_name" placeholder="给人看的名称，如 温度" />
        </el-form-item>
        <el-form-item label="单位" prop="unit">
          <el-input v-model="form.unit" placeholder="如 ℃、%、V，可留空" />
        </el-form-item>
        <el-form-item label="数据类型" prop="data_type">
          <el-select v-model="form.data_type" placeholder="请选择数据类型" style="width: 100%">
            <el-option label="数值 (number)" value="number" />
            <el-option label="布尔 (boolean)" value="boolean" />
            <el-option label="字符串 (string)" value="string" />
          </el-select>
        </el-form-item>
        <el-form-item prop="writable">
          <template #label>
            <span class="col-head">
              可下行设置
              <el-tooltip placement="top" effect="light">
                <template #content><span class="dir-tip">只声明数据点方向：关=设备仅上报（默认）；开=平台可下行设置。该开关不会自动生成下发按钮，实际下行操作在“设备控制”中配置。</span></template>
                <el-icon class="col-info"><InfoFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <el-switch v-model="form.writable" active-text="可下行设置" inactive-text="设备仅上报" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleAdd">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, nextTick, onMounted } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules, type TableInstance } from 'element-plus'
import { Plus, InfoFilled } from '@element-plus/icons-vue'
import { getProjectTags, addProjectTag, removeProjectTag, type ProjectTag } from '@/api/project'

const props = defineProps<{ projectId: number }>()

const tags = ref<ProjectTag[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref<FormInstance>()
const tableRef = ref<TableInstance>()

const form = reactive({
  tag_key: '',
  tag_name: '',
  unit: '',
  data_type: 'number',
  writable: false
})

const rules: FormRules = {
  tag_key: [{ required: true, message: '请输入标签键', trigger: 'blur' }],
  tag_name: [{ required: true, message: '请输入标签名称', trigger: 'blur' }],
  data_type: [{ required: true, message: '请选择数据类型', trigger: 'change' }]
}

function dataTypeLabel(t: string): string {
  switch (t) {
    case 'number': return '数值'
    case 'boolean': return '布尔'
    case 'string': return '字符串'
    default: return t || '数值'
  }
}

async function fetchTags() {
  if (!props.projectId) return
  loading.value = true
  try {
    const res = await getProjectTags(props.projectId)
    tags.value = res.data || []
  } catch {
    // 错误提示由拦截器处理
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.tag_key = ''
  form.tag_name = ''
  form.unit = ''
  form.data_type = 'number'
  form.writable = false
  formRef.value?.resetFields()
}

async function handleAdd() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    await addProjectTag(props.projectId, {
      tag_key: form.tag_key,
      tag_name: form.tag_name,
      unit: form.unit,
      data_type: form.data_type,
      writable: form.writable
    })
    ElMessage.success('数据点已添加')
    dialogVisible.value = false
    fetchTags()
  } catch {
    // 错误提示由拦截器处理
  } finally {
    submitting.value = false
  }
}

async function handleRemove(row: ProjectTag) {
  try {
    await ElMessageBox.confirm(`确定要删除数据点 “${row.tag_name || row.tag_key}” 吗？此操作不可恢复。`, '确认', {
      type: 'warning'
    })
  } catch {
    return
  }
  try {
    await removeProjectTag(props.projectId, row.id)
    ElMessage.success('已删除')
    fetchTags()
  } catch {
    // 错误提示由拦截器处理
  }
}

watch(() => props.projectId, fetchTags, { immediate: true })

// v-if 挂载首帧容器宽度可能未完成布局，挂载后强制重排，让表格撑满卡片
onMounted(() => {
  nextTick(() => tableRef.value?.doLayout())
  requestAnimationFrame(() => tableRef.value?.doLayout())
})

defineExpose({ refresh: fetchTags })
</script>

<style scoped>
.tag-manager {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.tm-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.tm-desc {
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--wd-text-secondary);
}
.col-head {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.col-info {
  font-size: 14px;
  color: var(--wd-text-placeholder);
  cursor: help;
}
.col-info:hover { color: var(--wd-primary); }
.dir-tip {
  font-size: 12px;
  line-height: 1.6;
  color: var(--wd-text-regular);
}
</style>
