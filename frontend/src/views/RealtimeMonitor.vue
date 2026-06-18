<template>
  <div class="monitor-container">
    <div class="task-list-panel">
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
    </div>

    <div class="log-panel">
      <div v-if="selectedTask" class="log-header">
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
        v-if="selectedTask"
        ref="logContainer"
        class="log-content"
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
          等待日志输出...
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { getRunningExecutions } from '../api'

const runningTasks = ref([])
const selectedTask = ref(null)
const currentStatus = ref('')
const logs = ref([])
const logContainer = ref(null)
const autoScroll = ref(true)
const ws = ref(null)
const pollTimer = ref(null)

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

const fetchRunningTasks = async () => {
  try {
    const data = await getRunningExecutions()
    runningTasks.value = data || []
  } catch (err) {
    console.error('获取运行中任务失败:', err)
  }
}

const selectTask = (task) => {
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

const handleMessage = (message) => {
  switch (message.type) {
    case 'status':
      currentStatus.value = message.data.status
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
  height: calc(100vh - 140px);
  gap: 20px;
}

.task-list-panel {
  width: 280px;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  display: flex;
  flex-direction: column;
  overflow: hidden;
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
</style>
