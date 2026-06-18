<template>
  <div class="monitor-container">
    <div class="monitor-header">
      <div class="header-left">
        <h2>实时监控</h2>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="openHistoryDrawer">
          <el-icon><History /></el-icon>
          <span>历史回放</span>
        </el-button>
      </div>
    </div>

    <div class="monitor-body">
      <div class="task-list-panel" :class="{ disabled: isPlaybackMode }">
        <div class="panel-header">
          <h3>运行中的任务</h3>
          <el-tag size="small" type="info">{{ runningTasks.length }} 个任务</el-tag>
        </div>
        <div class="task-list">
          <div
            v-for="task in runningTasks"
            :key="task.id"
            class="task-item"
            :class="{ active: selectedTask?.id === task.id }"
            @click="selectTask(task)"
          >
            <div class="task-name">{{ task.task_name }}</div>
            <div class="task-info">
              <el-tag :type="getStatusType(task.status)" size="small">
                {{ getStatusText(task.status) }}
              </el-tag>
              <span class="task-time">{{ formatTime(task.start_time) }}</span>
            </div>
          </div>
          <div v-if="runningTasks.length === 0" class="empty-tip">
            暂无运行中的任务
          </div>
        </div>
        <div v-if="isPlaybackMode" class="panel-mask"></div>
      </div>

      <div class="log-panel">
        <div v-if="isPlaybackMode && selectedHistoryExecution" class="log-header playback-header">
          <div class="log-title">
            <el-tag type="warning" size="small" effect="dark">回放中</el-tag>
            <span class="task-name">{{ selectedHistoryExecution.task_name }}</span>
            <el-tag :type="getStatusType(selectedHistoryExecution.status)" size="small" effect="dark">
              {{ getStatusText(selectedHistoryExecution.status) }}
            </el-tag>
          </div>
          <div class="log-actions">
            <el-button size="small" type="primary" @click="exitPlaybackMode">返回实时</el-button>
          </div>
        </div>
        <div v-else-if="selectedTask" class="log-header">
          <div class="log-title">
            <span class="task-name">{{ selectedTask.task_name }}</span>
            <el-tag :type="getStatusType(currentStatus)" size="small" effect="dark">
              {{ getStatusText(currentStatus) }}
            </el-tag>
          </div>
          <div class="log-actions">
            <el-button size="small" @click="clearLogs">清空日志</el-button>
            <el-button size="small" @click="scrollToBottom">滚动到底部</el-button>
          </div>
        </div>
        <div v-else class="empty-panel">
          <el-empty description="请选择左侧任务查看实时日志" />
        </div>

        <div
          v-if="(isPlaybackMode && selectedHistoryExecution) || selectedTask"
          ref="logContainer"
          class="log-content"
          :class="{ 'playback-mode': isPlaybackMode }"
          @scroll="handleScroll"
        >
          <div
            v-for="(log, index) in logs"
            :key="index"
            class="log-line"
            :class="log.stream"
          >
            <span class="log-prefix">[{{ log.stream }}]</span>
            <span class="log-text">{{ log.line }}</span>
          </div>
          <div v-if="logs.length === 0" class="no-logs">
            {{ isPlaybackMode ? '暂无历史日志' : '等待日志输出...' }}
          </div>
        </div>
      </div>
    </div>

    <el-drawer
      v-model="historyDrawerVisible"
      title="执行历史"
      direction="rtl"
      size="520px"
      @open="loadHistoryExecutions"
    >
      <div class="history-actions">
        <el-button type="primary" :disabled="!selectedHistoryExecution" @click="exitPlaybackMode">
          返回实时
        </el-button>
        <el-button @click="loadHistoryExecutions">
          <el-icon><Refresh /></el-icon>
          <span>刷新</span>
        </el-button>
      </div>

      <el-table
        v-loading="historyLoading"
        :data="historyExecutions"
        class="history-table"
        @row-click="selectHistoryExecution"
        stripe
        highlight-current-row
      >
        <el-table-column prop="task_name" label="任务名" min-width="120">
          <template #default="scope">
            <span class="history-task-name">{{ scope.row.task_name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="scope">
            <el-tag :type="getStatusType(scope.row.status)" size="small">
              {{ getStatusText(scope.row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="开始时间" width="160">
          <template #default="scope">
            {{ formatDateTime(scope.row.start_time) }}
          </template>
        </el-table-column>
        <el-table-column label="结束时间" width="160">
          <template #default="scope">
            {{ formatDateTime(scope.row.end_time) }}
          </template>
        </el-table-column>
        <el-table-column label="耗时(秒)" width="90" align="right">
          <template #default="scope">
            {{ formatDuration(scope.row.duration_ms) }}
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { ElMessage, ElNotification } from 'element-plus'
import { History, Refresh } from '@element-plus/icons-vue'
import { getRunningExecutions, getExecutionHistory, getExecutionDetail } from '../api'

const runningTasks = ref([])
const selectedTask = ref(null)
const currentStatus = ref('')
const logs = ref([])
const logContainer = ref(null)
const autoScroll = ref(true)
const ws = ref(null)
const pollTimer = ref(null)

const historyDrawerVisible = ref(false)
const historyLoading = ref(false)
const historyExecutions = ref([])
const selectedHistoryExecution = ref(null)
const isPlaybackMode = ref(false)

const lastAlertTimeMap = ref({})

const getStatusType = (status) => {
  switch (status) {
    case 'running': return 'primary'
    case 'success': return 'success'
    case 'failed': return 'danger'
    case 'timeout': return 'warning'
    default: return 'info'
  }
}

const getStatusText = (status) => {
  switch (status) {
    case 'running': return '运行中'
    case 'success': return '成功'
    case 'failed': return '失败'
    case 'timeout': return '超时'
    case 'pending': return '等待中'
    default: return status
  }
}

const formatTime = (time) => {
  if (!time) return ''
  const date = new Date(time)
  return date.toLocaleTimeString()
}

const formatDateTime = (time) => {
  if (!time) return '-'
  const date = new Date(time)
  return date.toLocaleString()
}

const formatDuration = (durationMs) => {
  if (durationMs == null) return '-'
  return (durationMs / 1000).toFixed(2)
}

const fetchRunningTasks = async () => {
  try {
    const data = await getRunningExecutions()
    runningTasks.value = data || []
  } catch (err) {
    console.error('获取运行中任务失败:', err)
  }
}

const selectTask = (task) => {
  if (isPlaybackMode.value) return
  if (selectedTask.value?.id === task.id) return

  closeWebSocket()
  selectedTask.value = task
  currentStatus.value = task.status
  logs.value = []
  autoScroll.value = true

  connectWebSocket(task.task_name)
}

const connectWebSocket = (taskName) => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}/ws/executions`

  try {
    ws.value = new WebSocket(wsUrl)

    ws.value.onopen = () => {
      console.log('WebSocket连接已建立')
      ws.value.send(JSON.stringify({
        action: 'subscribe',
        task_name: taskName
      }))
    }

    ws.value.onmessage = (event) => {
      const message = JSON.parse(event.data)
      handleMessage(message)
    }

    ws.value.onerror = (error) => {
      console.error('WebSocket错误:', error)
      ElMessage.error('WebSocket连接错误')
    }

    ws.value.onclose = () => {
      console.log('WebSocket连接已关闭')
    }
  } catch (err) {
    console.error('创建WebSocket失败:', err)
    ElMessage.error('创建WebSocket连接失败')
  }
}

const shouldTriggerAlert = (taskName) => {
  const now = Date.now()
  const lastTime = lastAlertTimeMap.value[taskName] || 0
  if (now - lastTime < 10000) {
    return false
  }
  lastAlertTimeMap.value[taskName] = now
  return true
}

const handleAlertNotification = (message) => {
  const newStatus = message.data.status
  if (!['failed', 'timeout'].includes(newStatus)) return

  const taskName = message.task_name
  if (!shouldTriggerAlert(taskName)) return

  const isFailed = newStatus === 'failed'
  const notificationType = isFailed ? 'error' : 'warning'
  const content = isFailed ? '执行失败' : '执行超时'

  ElNotification({
    title: taskName,
    message: content,
    type: notificationType,
    duration: 8000,
    onClick: () => {
      if (isPlaybackMode.value) {
        exitPlaybackMode()
      }
      jumpToTaskLog(taskName, message.data.execution_id)
    }
  })
}

const jumpToTaskLog = (taskName, executionId) => {
  const task = runningTasks.value.find(t => t.task_name === taskName || t.id === executionId)
  if (task) {
    selectTask(task)
  } else {
    selectedTask.value = null
    currentStatus.value = ''
    logs.value = []
    ElMessage.info('该任务已结束，可在历史回放中查看')
  }
}

const handleMessage = (message) => {
  switch (message.type) {
    case 'status':
      const oldStatus = currentStatus.value
      currentStatus.value = message.data.status
      handleAlertNotification(message)
      if (message.data.old_status === 'running' &&
          ['success', 'failed', 'timeout'].includes(message.data.status)) {
        setTimeout(() => {
          if (selectedTask.value?.id === message.data.execution_id) {
            ElMessage.info(`任务 ${message.task_name} 执行结束: ${getStatusText(message.data.status)}`)
          }
        }, 0)
      }
      break
    case 'log':
      logs.value.push({
        stream: message.data.stream,
        line: message.data.line
      })
      if (autoScroll.value) {
        nextTick(() => {
          scrollToBottom()
        })
      }
      break
    case 'heartbeat':
      break
  }
}

const handleScroll = () => {
  if (!logContainer.value) return

  const container = logContainer.value
  const scrollPosition = container.scrollTop + container.clientHeight
  const scrollHeight = container.scrollHeight

  autoScroll.value = scrollHeight - scrollPosition <= 50
}

const scrollToBottom = () => {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
    autoScroll.value = true
  }
}

const clearLogs = () => {
  logs.value = []
}

const closeWebSocket = () => {
  if (ws.value) {
    if (selectedTask.value && ws.value.readyState === WebSocket.OPEN) {
      ws.value.send(JSON.stringify({
        action: 'unsubscribe',
        task_name: selectedTask.value.task_name
      }))
    }
    ws.value.close()
    ws.value = null
  }
}

const openHistoryDrawer = () => {
  historyDrawerVisible.value = true
}

const loadHistoryExecutions = async () => {
  historyLoading.value = true
  try {
    const data = await getExecutionHistory()
    historyExecutions.value = data || []
  } catch (err) {
    console.error('获取执行历史失败:', err)
    ElMessage.error('获取执行历史失败')
  } finally {
    historyLoading.value = false
  }
}

const selectHistoryExecution = async (row) => {
  selectedHistoryExecution.value = row

  closeWebSocket()

  isPlaybackMode.value = true
  logs.value = []

  try {
    const detail = await getExecutionDetail(row.id)
    const combinedLogs = []
    if (detail.stdout) {
      detail.stdout.split('\n').forEach(line => {
        if (line) combinedLogs.push({ stream: 'stdout', line })
      })
    }
    if (detail.stderr) {
      detail.stderr.split('\n').forEach(line => {
        if (line) combinedLogs.push({ stream: 'stderr', line })
      })
    }
    logs.value = combinedLogs
    nextTick(() => {
      scrollToBottom()
    })
  } catch (err) {
    console.error('获取执行详情失败:', err)
    ElMessage.error('获取执行详情失败')
  }
}

const exitPlaybackMode = () => {
  isPlaybackMode.value = false
  selectedHistoryExecution.value = null
  historyDrawerVisible.value = false
  logs.value = []

  if (selectedTask.value) {
    connectWebSocket(selectedTask.value.task_name)
  }
}

onMounted(() => {
  fetchRunningTasks()
  pollTimer.value = setInterval(fetchRunningTasks, 3000)
})

onBeforeUnmount(() => {
  if (pollTimer.value) {
    clearInterval(pollTimer.value)
  }
  closeWebSocket()
})

watch(selectedTask, (newVal, oldVal) => {
  if (oldVal && !newVal) {
    closeWebSocket()
  }
})
</script>

<style scoped>
.monitor-container {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 140px);
  gap: 16px;
}

.monitor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 4px;
}

.monitor-header h2 {
  margin: 0;
  font-size: 20px;
  color: #303133;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.monitor-body {
  display: flex;
  flex: 1;
  gap: 20px;
  min-height: 0;
}

.task-list-panel {
  width: 280px;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

.task-list-panel.disabled {
  pointer-events: none;
}

.panel-mask {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(255, 255, 255, 0.6);
  z-index: 10;
  cursor: not-allowed;
}

.panel-header {
  padding: 16px;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.panel-header h3 {
  margin: 0;
  font-size: 16px;
}

.task-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.task-item {
  padding: 12px;
  border-radius: 6px;
  cursor: pointer;
  margin-bottom: 8px;
  transition: all 0.2s;
  border: 1px solid transparent;
}

.task-item:hover {
  background: #f5f7fa;
}

.task-item.active {
  background: #ecf5ff;
  border-color: #409eff;
}

.task-name {
  font-weight: 500;
  margin-bottom: 6px;
  word-break: break-all;
}

.task-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
}

.task-time {
  color: #909399;
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: #909399;
}

.log-panel {
  flex: 1;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.log-header {
  padding: 16px;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.log-header.playback-header {
  background: #fdf6ec;
}

.log-title {
  display: flex;
  align-items: center;
  gap: 12px;
}

.log-title .task-name {
  font-size: 16px;
  font-weight: 600;
  margin: 0;
}

.log-content {
  flex: 1;
  background: #000;
  color: #fff;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.5;
  padding: 16px;
  overflow-y: auto;
  transition: background-color 0.3s;
}

.log-content.playback-mode {
  background: #1a1a2e;
}

.log-line {
  white-space: pre-wrap;
  word-break: break-all;
  margin-bottom: 2px;
}

.log-line.stdout .log-prefix {
  color: #67c23a;
}

.log-line.stderr .log-prefix {
  color: #f56c6c;
}

.log-prefix {
  margin-right: 8px;
  font-weight: bold;
}

.no-logs {
  text-align: center;
  color: #666;
  padding: 40px;
}

.empty-panel {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.log-actions {
  display: flex;
  gap: 8px;
}

.history-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}

.history-table {
  width: 100%;
}

.history-task-name {
  font-weight: 500;
}

:deep(.el-table__body tr) {
  cursor: pointer;
}

:deep(.el-table__body tr.current-row > td) {
  background-color: #ecf5ff !important;
}
</style>
