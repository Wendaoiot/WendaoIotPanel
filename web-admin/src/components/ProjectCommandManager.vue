<template>
  <div class="cmd-manager">
    <div class="cm-toolbar">
      <span class="cm-desc">
        项目通用快捷指令，对本项目<b>所有设备</b>生效；单设备专属指令仍在该设备的“设备控制”页配置。
        标记为危险命令后，执行前需要二次确认。
      </span>
      <el-button type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon>
        新建指令
      </el-button>
    </div>

    <el-table ref="tableRef" :data="commands" v-loading="loading" stripe border>
      <el-table-column prop="id" label="ID" width="72" align="center" />
      <el-table-column prop="name" label="按钮名称" min-width="150" show-overflow-tooltip />
      <el-table-column prop="tag_key" label="标签键" min-width="130" show-overflow-tooltip />
      <el-table-column prop="value" label="下发值" width="90" align="center" />
      <el-table-column prop="sort" label="排序" width="80" align="center" />
      <el-table-column label="危险" width="80" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.danger" type="danger" size="small">高危</el-tag>
          <span v-else>—</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140" align="center" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="primary" link @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" link @click="handleRemove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑指令' : '新建指令'" width="500px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="92px">
        <el-form-item label="按钮名称" prop="name">
          <el-input v-model="form.name" placeholder="如：开继电器1" />
        </el-form-item>
        <el-form-item label="标签键" prop="tag_key">
          <el-input v-model="form.tag_key" placeholder="如 relay1（须与设备固件约定一致）" />
        </el-form-item>
        <el-form-item label="下发值" prop="value">
          <el-input-number v-model="form.value" :min="0" :max="65535" controls-position="right" />
        </el-form-item>
        <el-form-item label="排序" prop="sort">
          <el-input-number v-model="form.sort" :min="0" :max="9999" controls-position="right" />
          <span class="field-inline-hint">数值越小越靠前</span>
        </el-form-item>
        <el-form-item label="危险命令">
          <el-switch v-model="form.danger" />
          <span class="field-inline-hint">重启/恢复出厂等操作建议开启，执行前二次确认</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, nextTick, onMounted } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules, type TableInstance } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  listCommands, createCommand, updateCommand, deleteCommand,
  type ControlCommand
} from '@/api/device'

const props = defineProps<{ projectId: number }>()

const commands = ref<ControlCommand[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref<FormInstance>()
const tableRef = ref<TableInstance>()
const editing = ref<ControlCommand | null>(null)

const form = reactive({
  name: '',
  tag_key: '',
  value: 0,
  sort: 0,
  danger: false
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入按钮名称', trigger: 'blur' }],
  tag_key: [{ required: true, message: '请输入标签键', trigger: 'blur' }]
}

async function fetchCommands() {
  if (!props.projectId) return
  loading.value = true
  try {
    // 不传 device_id：只返回 device_id 为空的项目通用指令
    const res = await listCommands(props.projectId)
    commands.value = (res.data || []).filter(c => !c.device_id)
  } catch {
    // 错误提示由拦截器处理
  } finally {
    loading.value = false
  }
}

function resetForm() {
  editing.value = null
  form.name = ''
  form.tag_key = ''
  form.value = 0
  form.sort = 0
  form.danger = false
  formRef.value?.resetFields()
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: ControlCommand) {
  editing.value = row
  form.name = row.name
  form.tag_key = row.tag_key
  form.value = row.value
  form.sort = row.sort ?? 0
  form.danger = !!row.danger
  dialogVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    if (editing.value) {
      await updateCommand(editing.value.id, {
        name: form.name.trim(),
        tag_key: form.tag_key.trim(),
        value: form.value,
        sort: form.sort,
        danger: form.danger,
        device_id: ''
      })
      ElMessage.success('已保存')
    } else {
      await createCommand({
        project_id: props.projectId,
        name: form.name.trim(),
        tag_key: form.tag_key.trim(),
        value: form.value,
        sort: form.sort,
        danger: form.danger
      })
      ElMessage.success('指令已创建')
    }
    dialogVisible.value = false
    fetchCommands()
  } catch {
    // 错误提示由拦截器处理
  } finally {
    submitting.value = false
  }
}

async function handleRemove(row: ControlCommand) {
  try {
    await ElMessageBox.confirm(`确定删除指令【${row.name}】吗？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteCommand(row.id)
    ElMessage.success('已删除')
    commands.value = commands.value.filter(c => c.id !== row.id)
  } catch {
    // 错误提示由拦截器处理
  }
}

watch(() => props.projectId, fetchCommands, { immediate: true })

// v-if 挂载首帧容器宽度可能未完成布局，挂载后强制重排，让表格撑满卡片
onMounted(() => {
  nextTick(() => tableRef.value?.doLayout())
  requestAnimationFrame(() => tableRef.value?.doLayout())
})

defineExpose({ refresh: fetchCommands })
</script>

<style scoped>
.cmd-manager {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.cm-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.cm-desc {
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--wd-text-secondary);
}
.field-inline-hint {
  margin-left: 10px;
  font-size: 12px;
  color: var(--wd-text-secondary);
}
</style>
