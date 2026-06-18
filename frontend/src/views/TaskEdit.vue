<template>
  <div class="task-edit">
    <el-card>
      <template #header>
        <span class="card-title">{{ isEdit ? '编辑任务' : '新建任务' }}</span>
      </template>

      <el-form
        :model="form"
        :rules="rules"
        ref="formRef"
        label-width="140px"
      >
        <el-form-item label="任务名称" prop="name">
          <el-input
            v-model="form.name"
            :disabled="isEdit"
            placeholder="请输入任务名称"
          />
        </el-form-item>

        <el-form-item label="Cron表达式" prop="cron_expr">
          <el-input
            v-model="form.cron_expr"
            placeholder="*/5 * * * * 或 */5 * * * * * *"
            @input="previewCron"
          />
          <div style="margin-top: 8px">
            <el-alert
              v-if="previewResult.valid"
              type="success"
              :closable="false"
              show-icon
            >
              <template #title>
                <div>未来5次触发时间：</div>
                <div
                  v-for="(time, index) in previewResult.times"
                  :key="index"
                  style="margin-left: 20px"
                >
                  {{ index + 1 }}. {{ formatTime(time) }}
                </div>
              </template>
            </el-alert>
            <el-alert
              v-else-if="previewResult.error"
              type="error"
              :closable="false"
              show-icon
              :title="previewResult.error"
            />
          </div>
        </el-form-item>

        <el-form-item label="执行命令" prop="command">
          <el-input
            v-model="form.command"
            type="textarea"
            :rows="3"
            placeholder="Shell命令,如 echo hello"
          />
        </el-form-item>

        <el-form-item label="超时秒数">
          <el-input-number
            v-model="form.timeout_sec"
            :min="1"
            :max="86400"
          />
        </el-form-item>

        <el-form-item label="超时处理策略">
          <el-select v-model="form.timeout_strategy" style="width: 360px">
            <el-option label="强制终止并标记失败(默认)" value="kill_and_fail">
              <div style="font-weight: 500">强制终止并标记失败</div>
              <div style="font-size: 12px; color: #909399; margin-top: 2px">超时后立即终止任务执行,将状态标记为失败</div>
            </el-option>
            <el-option label="等待至自然结束再标记超时" value="wait_and_mark">
              <div style="font-weight: 500">等待至自然结束再标记超时</div>
              <div style="font-size: 12px; color: #909399; margin-top: 2px">不强制终止,等任务自然结束后,将状态标记为超时</div>
            </el-option>
            <el-option label="发送告警但不终止继续等待" value="alert_and_wait">
              <div style="font-weight: 500">发送告警但不终止继续等待</div>
              <div style="font-size: 12px; color: #909399; margin-top: 2px">超时时立即发送告警,但不终止任务,继续等待至自然结束</div>
            </el-option>
          </el-select>
        </el-form-item>

        <el-form-item label="最大重试次数">
          <el-input-number
            v-model="form.max_retries"
            :min="0"
            :max="10"
          />
        </el-form-item>

        <el-form-item label="重试间隔策略">
          <el-radio-group v-model="form.retry_strategy">
            <el-radio value="fixed">固定间隔</el-radio>
            <el-radio value="exponential">指数退避</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="重试间隔秒数">
          <el-input-number
            v-model="form.retry_interval_sec"
            :min="1"
            :max="3600"
          />
        </el-form-item>

        <el-form-item label="优先级">
          <el-input-number
            v-model="form.priority"
            :min="1"
            :max="10"
          />
        </el-form-item>

        <el-form-item label="依赖列表">
          <el-select
            v-model="form.dependencies"
            multiple
            placeholder="选择前置依赖任务"
            style="width: 100%"
          >
            <el-option
              v-for="name in availableDependencies"
              :key="name"
              :label="name"
              :value="name"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="触发条件">
          <el-radio-group v-model="form.trigger_condition">
            <el-radio value="all_success">全部成功</el-radio>
            <el-radio value="any_success">任一成功</el-radio>
            <el-radio value="any_complete">任一完成</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="补偿策略">
          <el-select
            v-model="form.compensation"
            style="width: 200px"
          >
            <el-option label="跳过" value="skip" />
            <el-option label="立即执行" value="execute_once" />
            <el-option label="排队等待" value="queue" />
          </el-select>
        </el-form-item>

        <el-form-item label="标签">
          <el-input
            v-model="tagsInput"
            placeholder="逗号分隔,如:ETL,报表"
            @blur="parseTags"
          />
        </el-form-item>

        <el-form-item label="是否启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>

        <el-form-item label="是否启用告警">
          <el-switch v-model="form.alert_enabled" />
        </el-form-item>

        <el-form-item>
          <el-button @click="handleCancel">取消</el-button>
          <el-button type="primary" @click="submitForm">确定</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import * as api from '@/api'
import dayjs from 'dayjs'

const route = useRoute()
const router = useRouter()

const isEdit = computed(() => !!route.params.name)

const formRef = ref(null)

const form = ref({
  name: '',
  cron_expr: '*/5 * * * *',
  command: '',
  timeout_sec: 60,
  timeout_strategy: 'kill_and_fail',
  max_retries: 0,
  retry_strategy: 'fixed',
  retry_interval_sec: 60,
  priority: 5,
  dependencies: [],
  trigger_condition: 'all_success',
  compensation: 'skip',
  tags: [],
  enabled: true,
  alert_enabled: true,
})

const tagsInput = ref('')

const rules = {
  name: [
    { required: true, message: '请输入任务名称', trigger: 'blur' },
  ],
  cron_expr: [
    { required: true, message: '请输入Cron表达式', trigger: 'blur' },
  ],
  command: [
    { required: true, message: '请输入执行命令', trigger: 'blur' },
  ],
}

const allTaskNames = ref([])

const availableDependencies = computed(() => {
  return allTaskNames.value.filter((name) => name !== form.value.name)
})

const previewResult = ref({
  valid: false,
  times: [],
  error: '',
})

let previewTimer = null

function previewCron() {
  if (previewTimer) {
    clearTimeout(previewTimer)
  }
  if (!form.value.cron_expr) {
    previewResult.value = { valid: false, times: [], error: '' }
    return
  }
  previewTimer = setTimeout(async () => {
    try {
      const data = await api.previewCron(form.value.cron_expr, 5)
      previewResult.value = data
    } catch (err) {
      previewResult.value = {
        valid: false,
        times: [],
        error: err.message || 'Cron表达式无效',
      }
    }
  }, 300)
}

function parseTags() {
  if (!tagsInput.value) {
    form.value.tags = []
    return
  }
  form.value.tags = tagsInput.value
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '')
}

function formatTime(t) {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

async function loadAllTaskNames() {
  try {
    const tasks = await api.getTasks()
    allTaskNames.value = (tasks || []).map((t) => t.name)
  } catch (err) {
    ElMessage.error(err.message || '加载任务列表失败')
  }
}

async function loadTaskData() {
  try {
    const data = await api.getTask(route.params.name)
    form.value = {
      name: data.name || '',
      cron_expr: data.cron_expr || '*/5 * * * *',
      command: data.command || '',
      timeout_sec: data.timeout_sec ?? 60,
      timeout_strategy: data.timeout_strategy || 'kill_and_fail',
      max_retries: data.max_retries ?? 0,
      retry_strategy: data.retry_strategy || 'fixed',
      retry_interval_sec: data.retry_interval_sec ?? 60,
      priority: data.priority ?? 5,
      dependencies: data.dependencies || [],
      trigger_condition: data.trigger_condition || 'all_success',
      compensation: data.compensation || 'skip',
      tags: data.tags || [],
      enabled: data.enabled ?? true,
      alert_enabled: data.alert_enabled ?? true,
    }
    tagsInput.value = (data.tags || []).join(', ')
    previewCron()
  } catch (err) {
    ElMessage.error(err.message || '加载任务数据失败')
  }
}

async function submitForm() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  parseTags()

  try {
    if (isEdit.value) {
      await api.updateTask(route.params.name, form.value)
      ElMessage.success('更新成功')
    } else {
      await api.createTask(form.value)
      ElMessage.success('创建成功')
    }
    router.push('/tasks')
  } catch (err) {
    ElMessage.error(err.message || '保存失败')
  }
}

function handleCancel() {
  router.push('/tasks')
}

onMounted(async () => {
  await loadAllTaskNames()
  if (isEdit.value) {
    await loadTaskData()
  } else {
    previewCron()
  }
})
</script>

<style scoped>
.task-edit {
  padding: 20px;
  max-width: 900px;
}

.card-title {
  font-size: 18px;
  font-weight: 600;
}
</style>
