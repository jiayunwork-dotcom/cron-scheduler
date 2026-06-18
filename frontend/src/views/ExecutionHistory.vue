<template>
  <el-card>
    <template #header>
      <span>执行历史</span>
    </template>

    <div class="filter-bar">
      <el-select
        v-model="filterTaskName"
        placeholder="选择任务名称"
        clearable
        style="width: 200px; margin-right: 12px"
      >
        <el-option
          v-for="name in allTaskNames"
          :key="name"
          :label="name"
          :value="name"
        />
      </el-select>
      <el-date-picker
        v-model="startTime"
        type="datetime"
        placeholder="开始时间"
        style="margin-right: 12px"
      />
      <el-date-picker
        v-model="endTime"
        type="datetime"
        placeholder="结束时间"
        style="margin-right: 12px"
      />
      <el-button type="primary" @click="loadData">查询</el-button>
      <el-button @click="resetFilter">重置</el-button>
    </div>

    <el-table :data="executions" stripe style="width: 100%; margin-top: 16px">
      <el-table-column label="ID" width="100">
        <template #default="{ row }">
          {{ String(row.id).slice(0, 8) }}
        </template>
      </el-table-column>
      <el-table-column prop="task_name" label="任务名称" />
      <el-table-column label="触发类型" width="100">
        <template #default="{ row }">
          <el-tag :type="getTriggerTypeTag(row.trigger_type).type">
            {{ getTriggerTypeTag(row.trigger_type).text }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="触发时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.trigger_time) }}
        </template>
      </el-table-column>
      <el-table-column label="开始时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.start_time) }}
        </template>
      </el-table-column>
      <el-table-column label="结束时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.end_time) }}
        </template>
      </el-table-column>
      <el-table-column label="耗时(ms)" width="100">
        <template #default="{ row }">
          {{ formatDuration(row.duration_ms) }}
        </template>
      </el-table-column>
      <el-table-column label="结果" width="100">
        <template #default="{ row }">
          <el-tag :type="getStatusTag(row.status).type">
            {{ getStatusTag(row.status).text }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="exit_code" label="退出码" width="80" />
      <el-table-column label="操作" width="100">
        <template #default="{ row }">
          <el-button size="small" @click="openDetail(row)">详情</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="currentPage"
      v-model:page-size="pageSize"
      :total="total"
      :page-sizes="[10, 20, 50, 100]"
      layout="total, sizes, prev, pager, next, jumper"
      style="margin-top: 16px; justify-content: flex-end"
      @size-change="loadData"
      @current-change="loadData"
    />

    <el-dialog
      v-model="detailVisible"
      title="执行详情"
      width="800px"
    >
      <el-descriptions :column="2" border v-if="currentExecution">
        <el-descriptions-item label="任务名">
          {{ currentExecution.task_name }}
        </el-descriptions-item>
        <el-descriptions-item label="触发类型">
          <el-tag :type="getTriggerTypeTag(currentExecution.trigger_type).type">
            {{ getTriggerTypeTag(currentExecution.trigger_type).text }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="触发时间">
          {{ formatTime(currentExecution.trigger_time) }}
        </el-descriptions-item>
        <el-descriptions-item label="开始时间">
          {{ formatTime(currentExecution.start_time) }}
        </el-descriptions-item>
        <el-descriptions-item label="结束时间">
          {{ formatTime(currentExecution.end_time) }}
        </el-descriptions-item>
        <el-descriptions-item label="耗时">
          {{ formatDuration(currentExecution.duration_ms) }} ms
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusTag(currentExecution.status).type">
            {{ getStatusTag(currentExecution.status).text }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="退出码">
          {{ currentExecution.exit_code ?? '-' }}
        </el-descriptions-item>
        <el-descriptions-item label="重试次数">
          {{ currentExecution.retry_count }}
        </el-descriptions-item>
        <el-descriptions-item label="错误信息">
          {{ currentExecution.error_message || '-' }}
        </el-descriptions-item>
      </el-descriptions>

      <el-tabs style="margin-top: 16px">
        <el-tab-pane label="标准输出" name="stdout">
          <pre class="log-area">{{ currentExecution?.stdout || '(无输出)' }}</pre>
        </el-tab-pane>
        <el-tab-pane label="标准错误" name="stderr">
          <pre class="log-area log-stderr">{{ currentExecution?.stderr || '(无错误输出)' }}</pre>
        </el-tab-pane>
      </el-tabs>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import * as api from '@/api'
import dayjs from 'dayjs'

const executions = ref([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)
const filterTaskName = ref('')
const startTime = ref(null)
const endTime = ref(null)

const detailVisible = ref(false)
const currentExecution = ref(null)

const allTaskNames = ref([])

const loadData = async () => {
  try {
    const params = {
      page: currentPage.value,
      page_size: pageSize.value
    }
    if (filterTaskName.value) {
      params.task_name = filterTaskName.value
    }
    if (startTime.value) {
      params.start_time = dayjs(startTime.value).toISOString()
    }
    if (endTime.value) {
      params.end_time = dayjs(endTime.value).toISOString()
    }

    const data = await api.getExecutions(params)
    executions.value = data.items
    total.value = data.total
  } catch (error) {
    console.error('加载执行历史失败:', error)
  }
}

const loadAllTaskNames = async () => {
  try {
    const tasks = await api.getTasks()
    allTaskNames.value = tasks.map((t) => t.name)
  } catch (error) {
    console.error('加载任务列表失败:', error)
  }
}

const formatDuration = (ms) => {
  if (ms == null) return '-'
  return ms.toString()
}

const formatTime = (t) => {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

const openDetail = async (exec) => {
  try {
    const data = await api.getExecution(exec.id)
    currentExecution.value = data
    detailVisible.value = true
  } catch (error) {
    console.error('加载执行详情失败:', error)
  }
}

const resetFilter = () => {
  filterTaskName.value = ''
  startTime.value = null
  endTime.value = null
  currentPage.value = 1
  loadData()
}

const getStatusTag = (status) => {
  const map = {
    success: { type: 'success', text: '成功' },
    failed: { type: 'danger', text: '失败' },
    running: { type: 'primary', text: '运行中' },
    timeout: { type: 'warning', text: '超时' },
    skipped: { type: 'info', text: '跳过' },
    interrupted: { type: '', text: '中断' },
    pending: { type: 'warning', text: '等待中' }
  }
  return map[status] || { type: 'info', text: status || '未知' }
}

const getTriggerTypeTag = (type) => {
  const map = {
    cron: { type: '', text: '定时' },
    manual: { type: 'primary', text: '手动' },
    compensation: { type: 'warning', text: '补偿' },
    skipped: { type: 'info', text: '跳过' }
  }
  return map[type] || { type: 'info', text: type || '未知' }
}

onMounted(() => {
  loadData()
  loadAllTaskNames()
})
</script>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.log-area {
  font-family: 'Consolas', 'Monaco', monospace;
  background: #f5f5f5;
  padding: 10px;
  max-height: 300px;
  overflow: auto;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}

.log-stderr {
  color: #ef4444;
}
</style>
