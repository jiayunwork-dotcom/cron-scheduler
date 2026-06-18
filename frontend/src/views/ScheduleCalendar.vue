<template>
  <el-card>
    <template #header>
      <span>调度日历</span>
    </template>

    <div class="filter-bar">
      <el-date-picker
        v-model="dateRange"
        type="daterange"
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        value-format="YYYY-MM-DD"
        style="width: 320px; margin-right: 12px"
        @change="handleDateChange"
      />
      <el-select
        v-model="filterTaskName"
        placeholder="按任务名过滤"
        clearable
        style="width: 220px; margin-right: 12px"
      >
        <el-option
          v-for="name in allTaskNames"
          :key="name"
          :label="name"
          :value="name"
        />
      </el-select>
      <el-button type="primary" @click="loadData">刷新</el-button>
      <el-button @click="goThisWeek">本周</el-button>
    </div>

    <div class="stats-cards" style="margin-top: 16px; margin-bottom: 16px">
      <el-card class="stat-card">
        <div class="stat-label">总执行次数</div>
        <div class="stat-value">{{ stats.totalCount }}</div>
      </el-card>
      <el-card class="stat-card">
        <div class="stat-label">成功率</div>
        <div class="stat-value success">{{ stats.successRate }}%</div>
      </el-card>
      <el-card class="stat-card">
        <div class="stat-label">平均耗时</div>
        <div class="stat-value">{{ stats.avgDuration }}s</div>
      </el-card>
      <el-card class="stat-card">
        <div class="stat-label">失败任务 Top3</div>
        <div class="stat-failed-list">
          <div
            v-for="(item, index) in stats.topFailedTasks"
            :key="item.name"
            class="stat-failed-item"
          >
            <span class="stat-failed-rank">{{ index + 1 }}</span>
            <span class="stat-failed-name">{{ item.name }}</span>
            <span class="stat-failed-count">{{ item.count }}次</span>
          </div>
          <div v-if="stats.topFailedTasks.length === 0" class="stat-failed-empty">
            暂无失败任务
          </div>
        </div>
      </el-card>
    </div>

    <div class="legend-bar" style="margin-top: 12px; margin-bottom: 12px">
      <span class="legend-label">状态图例:</span>
      <span class="legend-item"><span class="legend-color success"></span>成功</span>
      <span class="legend-item"><span class="legend-color failed"></span>失败</span>
      <span class="legend-item"><span class="legend-color timeout"></span>超时</span>
      <span class="legend-item"><span class="legend-color other"></span>其他</span>
    </div>

    <div class="calendar-container" v-loading="loading">
      <div class="calendar-grid">
        <div class="calendar-header">
          <div class="time-axis-header">时间</div>
          <div
            class="day-header"
            v-for="day in dateList"
            :key="day.dateStr"
            :class="{ 'is-today': day.isToday, 'is-weekend': day.isWeekend }"
          >
            <div class="day-weekday">{{ day.weekday }}</div>
            <div class="day-date">{{ day.dateStr }}</div>
          </div>
        </div>

        <div class="calendar-body">
          <div class="time-axis">
            <div
              class="time-cell"
              v-for="h in 24"
              :key="h - 1"
              :style="{ height: hourHeight + 'px', lineHeight: hourHeight + 'px' }"
            >
              {{ formatHour(h - 1) }}
            </div>
          </div>

          <div class="day-columns">
            <div
              class="day-column"
              v-for="day in dateList"
              :key="day.dateStr"
              :class="{ 'is-today': day.isToday, 'is-weekend': day.isWeekend }"
            >
              <div
                class="hour-grid"
                v-for="h in 24"
                :key="h - 1"
                :style="{ height: hourHeight + 'px' }"
              ></div>

              <div class="executions-layer">
                <template v-for="slot in getDaySlots(day.dateStr)" :key="slot.key">
                  <template v-if="slot.type === 'block'">
                    <el-popover
                      :width="280"
                      placement="right"
                      trigger="hover"
                    >
                      <template #reference>
                        <div
                          class="execution-block"
                          :class="[
                            'status-' + slot.exec.status,
                            { 'is-selected': selectedExecutionIds.has(slot.exec.id) }
                          ]"
                          :style="{
                            top: slot.top + 'px',
                            height: slot.height + 'px',
                            left: slot.left + '%',
                            width: slot.width + '%',
                            backgroundColor: getStatusColor(slot.exec.status)
                          }"
                          @click.stop="handleBlockClick(slot.exec)"
                        >
                          <span class="block-text">{{ getTaskShortName(slot.exec.task_name) }}</span>
                        </div>
                      </template>
                      <div class="popover-content">
                        <div class="popover-row">
                          <span class="popover-label">任务名:</span>
                          <span class="popover-value">{{ slot.exec.task_name }}</span>
                        </div>
                        <div class="popover-row">
                          <span class="popover-label">状态:</span>
                          <el-tag size="small" :type="getStatusTagType(slot.exec.status)">
                            {{ getStatusText(slot.exec.status) }}
                          </el-tag>
                        </div>
                        <div class="popover-row">
                          <span class="popover-label">开始时间:</span>
                          <span class="popover-value">{{ formatDateTime(slot.exec.start_time) }}</span>
                        </div>
                        <div class="popover-row">
                          <span class="popover-label">结束时间:</span>
                          <span class="popover-value">{{ formatDateTime(slot.exec.end_time) }}</span>
                        </div>
                        <div class="popover-row">
                          <span class="popover-label">耗时:</span>
                          <span class="popover-value">{{ formatDurationReadable(slot.exec.duration_ms) }}</span>
                        </div>
                        <div class="popover-row">
                          <span class="popover-label">触发类型:</span>
                          <span class="popover-value">{{ getTriggerTypeText(slot.exec.trigger_type) }}</span>
                        </div>
                      </div>
                    </el-popover>
                  </template>
                  <template v-else>
                    <el-popover
                      :width="320"
                      placement="right"
                      trigger="hover"
                    >
                      <template #reference>
                        <div
                          class="execution-block more-block"
                          :style="{
                            top: slot.top + 'px',
                            height: slot.height + 'px',
                            left: slot.left + '%',
                            width: slot.width + '%'
                          }"
                        >
                          <span class="block-text">+{{ slot.count }}</span>
                        </div>
                      </template>
                      <div class="popover-list">
                        <div
                          class="popover-list-item"
                          v-for="e in slot.execs"
                          :key="e.id"
                        >
                          <div class="popover-row">
                            <span
                              class="status-dot"
                              :style="{ backgroundColor: getStatusColor(e.status) }"
                            ></span>
                            <span class="popover-value strong">{{ e.task_name }}</span>
                          </div>
                          <div class="popover-row small">
                            <span class="popover-label">{{ formatDateTime(e.start_time) }}</span>
                            <span class="popover-sep">~</span>
                            <span class="popover-label">{{ formatDateTime(e.end_time) }}</span>
                          </div>
                          <div class="popover-row small">
                            <span class="popover-label">耗时: {{ formatDurationReadable(e.duration_ms) }}</span>
                            <el-tag size="small" :type="getStatusTagType(e.status)" style="margin-left: 8px">
                              {{ getStatusText(e.status) }}
                            </el-tag>
                          </div>
                        </div>
                      </div>
                    </el-popover>
                  </template>
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="batch-action-bar" v-if="hasSelection">
      <div class="batch-action-info">
        已选中 <strong>{{ selectedExecutionIds.size }}</strong> 条记录
      </div>
      <div class="batch-action-buttons">
        <el-button type="primary" @click="handleBatchRerun">批量重新执行</el-button>
        <el-button @click="handleExportCSV">导出CSV</el-button>
        <el-button @click="clearSelection">关闭</el-button>
      </div>
    </div>
  </el-card>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import * as api from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'
import isoWeek from 'dayjs/plugin/isoWeek'

dayjs.extend(isoWeek)

const loading = ref(false)
const executions = ref([])
const allTaskNames = ref([])
const filterTaskName = ref('')
const selectedExecutionIds = ref(new Set())
const shiftPressed = ref(false)
const batchRerunDialogVisible = ref(false)

const hourHeight = 40
const MIN_DURATION_MIN = 3
const MAX_PARALLEL = 4

const defaultWeekRange = () => {
  const monday = dayjs().isoWeekday(1)
  const sunday = dayjs().isoWeekday(7)
  return [monday.format('YYYY-MM-DD'), sunday.format('YYYY-MM-DD')]
}

const dateRange = ref(defaultWeekRange())

const dateList = computed(() => {
  const list = []
  if (!dateRange.value || dateRange.value.length !== 2) return list
  const start = dayjs(dateRange.value[0])
  const end = dayjs(dateRange.value[1])
  const today = dayjs().format('YYYY-MM-DD')
  const weekdays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
  let cur = start
  while (cur.isBefore(end) || cur.isSame(end, 'day')) {
    const dateStr = cur.format('YYYY-MM-DD')
    const dayNum = cur.day()
    list.push({
      dateStr,
      weekday: weekdays[dayNum],
      isToday: dateStr === today,
      isWeekend: dayNum === 0 || dayNum === 6
    })
    cur = cur.add(1, 'day')
  }
  return list
})

const filteredExecutions = computed(() => {
  let list = executions.value
  if (filterTaskName.value) {
    list = list.filter(e => e.task_name === filterTaskName.value)
  }
  return list
})

const stats = computed(() => {
  const list = filteredExecutions.value
  const totalCount = list.length

  let successCount = 0
  let totalDurationMs = 0
  let durationCount = 0
  const failedTaskMap = {}

  for (const exec of list) {
    if (exec.status === 'success') {
      successCount++
    }
    if (exec.status === 'failed' || exec.status === 'timeout') {
      if (failedTaskMap[exec.task_name]) {
        failedTaskMap[exec.task_name]++
      } else {
        failedTaskMap[exec.task_name] = 1
      }
    }
    if (exec.duration_ms != null) {
      totalDurationMs += exec.duration_ms
      durationCount++
    }
  }

  const successRate = totalCount > 0
    ? ((successCount / totalCount) * 100).toFixed(1)
    : '0.0'

  const avgDuration = durationCount > 0
    ? ((totalDurationMs / durationCount) / 1000).toFixed(1)
    : '0.0'

  const topFailedTasks = Object.entries(failedTaskMap)
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 3)

  return {
    totalCount,
    successRate,
    avgDuration,
    topFailedTasks
  }
})

const selectedExecutions = computed(() => {
  return filteredExecutions.value.filter(e => selectedExecutionIds.value.has(e.id))
})

const hasSelection = computed(() => selectedExecutionIds.value.size > 0)

const executionsByDay = computed(() => {
  const map = {}
  for (const day of dateList.value) {
    map[day.dateStr] = []
  }
  for (const exec of filteredExecutions.value) {
    const start = exec.start_time ? dayjs(exec.start_time) : null
    if (!start) continue
    const dayStr = start.format('YYYY-MM-DD')
    if (map[dayStr]) {
      map[dayStr].push(exec)
    }
  }
  return map
})

const computeMinutesOfDay = (time) => {
  const t = dayjs(time)
  return t.hour() * 60 + t.minute() + t.second() / 60
}

const computeDurationMinutes = (exec) => {
  if (exec.duration_ms != null) {
    return exec.duration_ms / 60000
  }
  if (exec.start_time && exec.end_time) {
    return dayjs(exec.end_time).diff(dayjs(exec.start_time), 'minute', true)
  }
  return MIN_DURATION_MIN
}

const getTaskShortName = (name) => {
  if (!name) return ''
  return name.length > 6 ? name.slice(0, 6) : name
}

const formatHour = (h) => {
  return h.toString().padStart(2, '0') + ':00'
}

const getStatusColor = (status) => {
  switch (status) {
    case 'success':
      return '#10b981'
    case 'failed':
      return '#ef4444'
    case 'timeout':
      return '#f97316'
    default:
      return '#9ca3af'
  }
}

const getStatusTagType = (status) => {
  switch (status) {
    case 'success':
      return 'success'
    case 'failed':
      return 'danger'
    case 'timeout':
      return 'warning'
    default:
      return 'info'
  }
}

const getStatusText = (status) => {
  const map = {
    success: '成功',
    failed: '失败',
    running: '运行中',
    timeout: '超时',
    skipped: '跳过',
    interrupted: '中断',
    pending: '等待中'
  }
  return map[status] || status || '未知'
}

const getTriggerTypeText = (type) => {
  const map = {
    cron: '定时触发',
    manual: '手动触发',
    compensation: '补偿触发',
    skipped: '跳过触发'
  }
  return map[type] || type || '未知'
}

const formatDateTime = (t) => {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

const formatDurationReadable = (ms) => {
  if (ms == null) return '-'
  if (ms < 1000) return ms + 'ms'
  const totalSec = Math.floor(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  const parts = []
  if (h > 0) parts.push(h + 'h')
  if (m > 0) parts.push(m + 'm')
  if (s > 0 || parts.length === 0) parts.push(s + 's')
  return parts.join(' ')
}

const getOverlappingGroups = (execs) => {
  const intervals = execs.map(e => {
    const startMin = computeMinutesOfDay(e.start_time)
    const durationMin = Math.max(computeDurationMinutes(e), MIN_DURATION_MIN)
    const endMin = startMin + durationMin
    return { exec, startMin, endMin }
  })

  intervals.sort((a, b) => a.startMin - b.startMin)

  const groups = []
  for (const iv of intervals) {
    let placed = false
    for (const g of groups) {
      const last = g[g.length - 1]
      if (iv.startMin >= last.endMin) {
        g.push(iv)
        placed = true
        break
      }
    }
    if (!placed) {
      groups.push([iv])
    }
  }
  return groups
}

const getDaySlots = (dayStr) => {
  const execs = executionsByDay.value[dayStr] || []
  if (execs.length === 0) return []

  const groups = getOverlappingGroups(execs)
  const slots = []
  const perSlotHeight = (pct) => hourHeight * 24 * pct / 100

  for (const group of groups) {
    const parallelCount = Math.min(group.length, MAX_PARALLEL)
    const slotWidth = 100 / parallelCount

    for (let i = 0; i < parallelCount; i++) {
      const iv = group[i]
      const startPct = (iv.startMin / (24 * 60)) * 100
      const durationMin = iv.endMin - iv.startMin
      const heightPct = (durationMin / (24 * 60)) * 100

      const top = (startPct / 100) * hourHeight * 24
      const height = Math.max((heightPct / 100) * hourHeight * 24, (MIN_DURATION_MIN / (24 * 60)) * hourHeight * 24)

      slots.push({
        key: `${dayStr}-${iv.exec.id}-${i}`,
        type: 'block',
        exec: iv.exec,
        top,
        height,
        left: i * slotWidth,
        width: slotWidth - 1
      })
    }

    if (group.length > MAX_PARALLEL) {
      const restIvs = group.slice(MAX_PARALLEL)
      const avgStart = restIvs.reduce((s, iv) => s + iv.startMin, 0) / restIvs.length
      const maxEnd = Math.max(...restIvs.map(iv => iv.endMin))
      const startPct = (avgStart / (24 * 60)) * 100
      const durationMin = Math.max(maxEnd - avgStart, MIN_DURATION_MIN)
      const heightPct = (durationMin / (24 * 60)) * 100

      const top = (startPct / 100) * hourHeight * 24
      const height = Math.max((heightPct / 100) * hourHeight * 24, (MIN_DURATION_MIN / (24 * 60)) * hourHeight * 24)
      const slotWidth = 100 / MAX_PARALLEL

      slots.push({
        key: `${dayStr}-more-${group[0].exec.id}`,
        type: 'more',
        count: group.length - MAX_PARALLEL,
        execs: restIvs.map(iv => iv.exec),
        top,
        height,
        left: (MAX_PARALLEL - 1) * slotWidth,
        width: slotWidth - 1
      })
    }
  }

  return slots
}

const handleDateChange = () => {
  loadData()
}

const goThisWeek = () => {
  dateRange.value = defaultWeekRange()
  loadData()
}

const loadData = async () => {
  loading.value = true
  try {
    const params = {}
    if (dateRange.value && dateRange.value.length === 2) {
      params.start_date = dateRange.value[0]
      params.end_date = dateRange.value[1]
    }
    const data = await api.getCalendarExecutions(params)
    executions.value = data
  } catch (error) {
    console.error('加载日历数据失败:', error)
  } finally {
    loading.value = false
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

const handleBlockClick = (exec) => {
  if (shiftPressed.value) {
    const newSet = new Set(selectedExecutionIds.value)
    if (newSet.has(exec.id)) {
      newSet.delete(exec.id)
    } else {
      newSet.add(exec.id)
    }
    selectedExecutionIds.value = newSet
  } else {
    if (selectedExecutionIds.value.size > 0) {
      selectedExecutionIds.value = new Set()
    }
  }
}

const clearSelection = () => {
  selectedExecutionIds.value = new Set()
}

const handleBatchRerun = async () => {
  const selected = selectedExecutions.value
  if (selected.length === 0) {
    ElMessage.warning('请先选择执行记录')
    return
  }

  const taskListHtml = selected
    .map(e => `<div style="padding: 4px 0;">• ${e.task_name} (${formatDateTime(e.start_time)})</div>`)
    .join('')

  try {
    await ElMessageBox.confirm(
      `即将重新触发以下 ${selected.length} 个任务：<br/><br/>${taskListHtml}`,
      '批量重新执行确认',
      {
        dangerouslyUseHTMLString: true,
        confirmButtonText: '确认执行',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    let successCount = 0
    let failCount = 0

    for (const exec of selected) {
      try {
        await api.triggerTask(exec.task_name)
        successCount++
      } catch (err) {
        failCount++
      }
    }

    if (failCount === 0) {
      ElMessage.success(`成功触发 ${successCount} 个任务`)
    } else {
      ElMessage.warning(`成功触发 ${successCount} 个，失败 ${failCount} 个`)
    }

    clearSelection()
    loadData()
  } catch {
  }
}

const handleExportCSV = () => {
  const selected = selectedExecutions.value
  if (selected.length === 0) {
    ElMessage.warning('请先选择执行记录')
    return
  }

  const headers = ['task_name', 'status', 'start_time', 'end_time', 'duration_ms', 'trigger_type']
  const headerCn = ['任务名', '状态', '开始时间', '结束时间', '耗时(ms)', '触发类型']

  const rows = selected.map(exec => {
    return [
      exec.task_name || '',
      getStatusText(exec.status),
      exec.start_time ? formatDateTime(exec.start_time) : '',
      exec.end_time ? formatDateTime(exec.end_time) : '',
      exec.duration_ms != null ? exec.duration_ms : '',
      getTriggerTypeText(exec.trigger_type)
    ].map(v => `"${String(v).replace(/"/g, '""')}"`).join(',')
  })

  const csvContent = [headerCn.join(','), ...rows].join('\n')
  const BOM = '\uFEFF'
  const blob = new Blob([BOM + csvContent], { type: 'text/csv;charset=utf-8;' })

  const dateStr = dayjs().format('YYYY-MM-DD')
  const fileName = `executions_${dateStr}.csv`

  const link = document.createElement('a')
  const url = URL.createObjectURL(blob)
  link.setAttribute('href', url)
  link.setAttribute('download', fileName)
  link.style.visibility = 'hidden'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)

  ElMessage.success(`已导出 ${selected.length} 条记录`)
}

const handleKeyDown = (e) => {
  if (e.key === 'Shift') {
    shiftPressed.value = true
  }
  if (e.key === 'Escape' && hasSelection.value) {
    clearSelection()
  }
}

const handleKeyUp = (e) => {
  if (e.key === 'Shift') {
    shiftPressed.value = false
  }
}

onMounted(() => {
  loadData()
  loadAllTaskNames()
  window.addEventListener('keydown', handleKeyDown)
  window.addEventListener('keyup', handleKeyUp)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeyDown)
  window.removeEventListener('keyup', handleKeyUp)
})
</script>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.stats-cards {
  display: flex;
  gap: 16px;
}

.stat-card {
  flex: 1;
  min-width: 0;
}

.stat-label {
  font-size: 13px;
  color: #909399;
  margin-bottom: 8px;
}

.stat-value {
  font-size: 28px;
  font-weight: 600;
  color: #303133;
}

.stat-value.success {
  color: #67c23a;
}

.stat-failed-list {
  font-size: 13px;
}

.stat-failed-item {
  display: flex;
  align-items: center;
  padding: 4px 0;
  gap: 8px;
}

.stat-failed-rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background-color: #f56c6c;
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  flex-shrink: 0;
}

.stat-failed-item:nth-child(2) .stat-failed-rank {
  background-color: #e6a23c;
}

.stat-failed-item:nth-child(3) .stat-failed-rank {
  background-color: #909399;
}

.stat-failed-name {
  flex: 1;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stat-failed-count {
  color: #f56c6c;
  font-weight: 500;
  flex-shrink: 0;
}

.stat-failed-empty {
  color: #c0c4cc;
  font-size: 12px;
  padding: 8px 0;
}

.legend-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: 13px;
  color: #606266;
}

.legend-label {
  font-weight: 500;
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.legend-color {
  display: inline-block;
  width: 14px;
  height: 14px;
  border-radius: 3px;
}

.legend-color.success {
  background-color: #10b981;
}

.legend-color.failed {
  background-color: #ef4444;
}

.legend-color.timeout {
  background-color: #f97316;
}

.legend-color.other {
  background-color: #9ca3af;
}

.calendar-container {
  margin-top: 16px;
  overflow-x: auto;
  border: 1px solid #ebeef5;
  border-radius: 4px;
}

.calendar-grid {
  min-width: 100%;
  display: flex;
  flex-direction: column;
}

.calendar-header {
  display: flex;
  border-bottom: 1px solid #ebeef5;
  background-color: #fafafa;
  position: sticky;
  top: 0;
  z-index: 10;
}

.time-axis-header {
  flex: 0 0 70px;
  min-width: 70px;
  text-align: center;
  font-weight: 600;
  padding: 10px 0;
  border-right: 1px solid #ebeef5;
  color: #303133;
  background-color: #f5f7fa;
}

.day-header {
  flex: 1;
  text-align: center;
  padding: 8px 0;
  border-right: 1px solid #ebeef5;
  min-width: 140px;
}

.day-header:last-child {
  border-right: none;
}

.day-header.is-weekend {
  background-color: #fdf6ec;
}

.day-header.is-today {
  background-color: #ecf5ff;
}

.day-weekday {
  font-size: 13px;
  color: #606266;
}

.day-date {
  font-weight: 600;
  color: #303133;
  font-size: 14px;
}

.calendar-body {
  display: flex;
  position: relative;
}

.time-axis {
  flex: 0 0 70px;
  min-width: 70px;
  border-right: 1px solid #ebeef5;
  background-color: #f5f7fa;
}

.time-cell {
  border-bottom: 1px dashed #ebeef5;
  text-align: center;
  font-size: 12px;
  color: #909399;
  box-sizing: border-box;
}

.day-columns {
  flex: 1;
  display: flex;
  position: relative;
}

.day-column {
  flex: 1;
  position: relative;
  border-right: 1px solid #ebeef5;
  min-width: 140px;
}

.day-column:last-child {
  border-right: none;
}

.day-column.is-weekend {
  background-color: #fdfaf3;
}

.day-column.is-today {
  background-color: #f0f9ff;
}

.hour-grid {
  border-bottom: 1px dashed #ebeef5;
  box-sizing: border-box;
}

.executions-layer {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  pointer-events: none;
}

.execution-block {
  position: absolute;
  border-radius: 4px;
  padding: 2px 4px;
  box-sizing: border-box;
  overflow: hidden;
  cursor: pointer;
  opacity: 0.9;
  border: 1px solid rgba(0, 0, 0, 0.1);
  pointer-events: auto;
  transition: opacity 0.15s;
  z-index: 2;
}

.execution-block:hover {
  opacity: 1;
  z-index: 5;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
}

.execution-block.is-selected {
  outline: 2px dashed #409eff;
  outline-offset: 1px;
  z-index: 6;
}

.execution-block.more-block {
  background-color: #6b7280;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
}

.block-text {
  font-size: 11px;
  color: #fff;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  text-shadow: 0 1px 1px rgba(0, 0, 0, 0.2);
  display: block;
}

.more-block .block-text {
  text-shadow: none;
}

.popover-content {
  font-size: 13px;
}

.popover-row {
  display: flex;
  align-items: center;
  padding: 4px 0;
  gap: 8px;
}

.popover-row.small {
  font-size: 12px;
  color: #606266;
}

.popover-label {
  color: #606266;
  flex: 0 0 70px;
}

.popover-value {
  color: #303133;
  flex: 1;
  word-break: break-all;
}

.popover-value.strong {
  font-weight: 600;
}

.popover-sep {
  color: #c0c4cc;
  padding: 0 4px;
}

.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  flex-shrink: 0;
}

.popover-list {
  max-height: 360px;
  overflow-y: auto;
}

.popover-list-item {
  padding: 8px 4px;
  border-bottom: 1px solid #f0f0f0;
}

.popover-list-item:last-child {
  border-bottom: none;
}

.batch-action-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  background-color: #fff;
  border-top: 1px solid #ebeef5;
  box-shadow: 0 -2px 12px rgba(0, 0, 0, 0.1);
  padding: 12px 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  z-index: 1000;
}

.batch-action-info {
  font-size: 14px;
  color: #606266;
}

.batch-action-info strong {
  color: #409eff;
  font-size: 16px;
  margin: 0 4px;
}

.batch-action-buttons {
  display: flex;
  gap: 12px;
}
</style>
