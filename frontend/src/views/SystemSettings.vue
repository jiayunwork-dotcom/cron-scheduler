<template>
  <el-card>
    <template #header>
      <span>系统设置</span>
    </template>

    <el-form :model="form" label-width="200px" style="max-width: 700px">
      <el-form-item label="全局并发任务数">
        <el-input-number v-model="form.max_concurrent" :min="1" :max="50" />
      </el-form-item>

      <el-form-item label="默认超时秒数">
        <el-input-number v-model="form.default_timeout" :min="1" :max="86400" />
      </el-form-item>

      <el-form-item label="告警Webhook URL">
        <el-input
          v-model="form.webhook_url" placeholder="https://example.com/webhook" />
      </el-form-item>

      <el-form-item label="默认补偿策略">
        <el-select v-model="form.default_compensation" style="width: 200px">
          <el-option label="跳过" value="skip" />
          <el-option label="执行一次" value="execute_once" />
          <el-option label="入队" value="queue" />
        </el-select>
      </el-form-item>

      <el-form-item label="连续失败告警阈值">
        <el-input-number v-model="form.consecutive_failures" :min="1" :max="100" />
        <div style="color: #909399; font-size: 12px; margin-top: 4px">
          连续失败N次才告警
        </div>
      </el-form-item>

      <el-form-item label="告警静默期(分钟)">
        <el-input-number v-model="form.silent_minutes" :min="0" :max="1440" />
        <div style="color: #909399; font-size: 12px; margin-top: 4px">
          同一任务在此时间内不重复告警
        </div>
      </el-form-item>

      <el-form-item>
        <el-button type="primary" @click="saveSettings">保存设置</el-button>
        <el-button @click="testWebhook">测试Webhook</el-button>
      </el-form-item>
    </el-form>

    <el-card style="margin-top: 24px">
      <template #header>
        <div class="missed-header">
          <span>错过执行检测</span>
          <el-button type="primary" @click="runDetect">立即检测</el-button>
        </div>
      </template>

      <el-table :data="missedList" v-if="missedList.length > 0" stripe>
        <el-table-column prop="task_name" label="任务名" />
        <el-table-column label="应触发时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.scheduled_time) }}
          </template>
        </el-table-column>
        <el-table-column label="检测时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.detected_at) }}
          </template>
        </el-table-column>
        <el-table-column label="补偿策略" width="120">
          <template #default="{ row }">
            <el-tag>{{ getCompensationText(row.compensation) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" @click="compensateItem(row)">补偿</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-else description="暂无未补偿的错过执行" />
    </el-card>
  </el-card>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import * as api from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'

const form = reactive({
  max_concurrent: 5,
  default_timeout: 60,
  webhook_url: '',
  default_compensation: 'skip',
  consecutive_failures: 3,
  silent_minutes: 10
})

const missedList = ref([])

const loadSettings = async () => {
  try {
    const settings = await api.getSettings()
    if (settings.max_concurrent) {
      form.max_concurrent = parseInt(settings.max_concurrent) || 5
    }
    if (settings.default_timeout) {
      form.default_timeout = parseInt(settings.default_timeout) || 60
    }
    if (settings.webhook_url) {
      form.webhook_url = settings.webhook_url
    }
    if (settings.default_compensation) {
      form.default_compensation = settings.default_compensation || 'skip'
    }
    if (settings.consecutive_failures) {
      form.consecutive_failures = parseInt(settings.consecutive_failures) || 3
    }
    if (settings.silent_minutes) {
      form.silent_minutes = parseInt(settings.silent_minutes) || 10
    }
  } catch (error) {
    console.error('加载系统设置失败:', error)
  }
}

const saveSettings = async () => {
  try {
    await api.updateSettings({
      max_concurrent: String(form.max_concurrent),
      default_timeout: String(form.default_timeout),
      webhook_url: form.webhook_url,
      default_compensation: form.default_compensation,
      consecutive_failures: String(form.consecutive_failures),
      silent_minutes: String(form.silent_minutes)
    })
    ElMessage.success('设置保存成功')
  } catch (error) {
    ElMessage.error('保存失败: ' + error.message)
  }
}

const testWebhook = () => {
  ElMessage.info('Webhook测试功能将在任务失败时触发，实际发送需要真实失败场景')
}

const runDetect = async () => {
  try {
    await ElMessageBox.confirm(
      '确定要立即执行错过执行检测吗？检测过程中会自动触发补偿策略。',
      '确认检测',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    const data = await api.detectMissed()
    const count = Array.isArray(data) ? data.length : 0
    ElMessage.success(`检测完成，发现 ${count} 条错过执行`)
    loadMissed()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('检测错过执行失败:', error)
    }
  }
}

const loadMissed = async () => {
  try {
    const data = await api.getMissed()
    missedList.value = data.filter((item) => !item.compensated)
  } catch (error) {
    console.error('加载错过执行列表失败:', error)
  }
}

const compensateItem = () => {
  ElMessage.info('请点击"立即检测"按钮触发自动补偿')
}

const formatTime = (t) => {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

const getCompensationText = (type) => {
  const map = {
    skip: '跳过',
    execute_once: '执行一次',
    queue: '入队'
  }
  return map[type] || type || '未知'
}

onMounted(() => {
  loadSettings()
  loadMissed()
})
</script>

<style scoped>
.missed-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}
</style>
