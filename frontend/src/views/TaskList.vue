<template>
  <div class="task-list">
    <el-alert
      v-if="currentMissedList.length > 0"
      title="存在未补偿的错过执行"
      type="warning"
      :closable="false"
      show-icon
    >
      <template #default>
        <div v-for="(item, index) in currentMissedList" :key="index">
          {{ item.task_name }} - {{ formatTime(item.scheduled_time) }}
        </div>
      </template>
    </el-alert>

    <div class="toolbar">
      <el-input
        v-model="searchKeyword"
        placeholder="搜索任务名称"
        clearable
        style="width: 200px"
      />
      <el-select
        v-model="filterTag"
        placeholder="按标签筛选"
        clearable
        style="width: 180px"
      >
        <el-option
          v-for="tag in allTags"
          :key="tag"
          :label="tag"
          :value="tag"
        />
      </el-select>
      <el-select
        v-model="filterEnabled"
        placeholder="状态筛选"
        clearable
        style="width: 150px"
      >
        <el-option label="全部" value="" />
        <el-option label="启用" value="true" />
        <el-option label="暂停" value="false" />
        <el-option label="异常" value="error" />
      </el-select>
      <el-button @click="loadData">
        <el-icon><Refresh /></el-icon>
        刷新
      </el-button>
      <el-button type="primary" @click="$router.push('/tasks/new')">
        <el-icon><Plus /></el-icon>
        新建任务
      </el-button>
    </div>

    <el-table :data="pagedTasks" stripe style="width: 100%">
      <el-table-column prop="name" label="任务名称" min-width="150" />
      <el-table-column prop="cron_expr" label="Cron表达式" width="180" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="getStatusTag(row).type">
            {{ getStatusTag(row).text }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="标签" min-width="150">
        <template #default="{ row }">
          <el-tag
            v-for="tag in row.tags"
            :key="tag"
            size="small"
            style="margin-right: 4px"
          >
            {{ tag }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="priority" label="优先级" width="80" align="center" />
      <el-table-column label="最近执行时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.last_run_at) }}
        </template>
      </el-table-column>
      <el-table-column label="最近结果" width="100">
        <template #default="{ row }">
          <el-tag :type="getResultTag(row.last_result).type">
            {{ getResultTag(row.last_result).text }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="下次触发时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.next_run_at) }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="240" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="row.enabled"
            size="small"
            @click="handleToggleEnabled(row)"
          >
            暂停
          </el-button>
          <el-button
            v-else
            size="small"
            type="success"
            @click="handleToggleEnabled(row)"
          >
            启用
          </el-button>
          <el-button size="small" type="primary" @click="handleTrigger(row)">
            立即触发
          </el-button>
          <el-button size="small" @click="$router.push(`/tasks/${row.name}/edit`)">
            编辑
          </el-button>
          <el-popconfirm title="确认删除该任务?" @confirm="handleDelete(row)">
            <template #reference>
              <el-button size="small" type="danger">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <div class="pagination">
      <el-pagination
        layout="total, sizes, prev, pager, next, jumper"
        :total="totalTasks"
        :current-page="currentPage"
        :page-size="pageSize"
        :page-sizes="[10, 20, 50, 100]"
        @current-change="handlePageChange"
        @size-change="handleSizeChange"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh, Plus } from '@element-plus/icons-vue'
import * as api from '@/api'
import dayjs from 'dayjs'

const router = useRouter()

const tasks = ref([])
const searchKeyword = ref('')
const filterTag = ref('')
const filterEnabled = ref('')
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)
const missedMap = ref({})

const allTasks = ref([])

const allTags = computed(() => {
  const tagSet = new Set()
  allTasks.value.forEach((task) => {
    task.tags?.forEach((tag) => tagSet.add(tag))
  })
  return Array.from(tagSet)
})

const filteredTasks = computed(() => {
  let result = [...allTasks.value]

  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    result = result.filter((task) =>
      task.name.toLowerCase().includes(keyword)
    )
  }

  if (filterTag.value) {
    result = result.filter((task) =>
      task.tags?.includes(filterTag.value)
    )
  }

  if (filterEnabled.value === 'true') {
    result = result.filter((task) => task.enabled)
  } else if (filterEnabled.value === 'false') {
    result = result.filter((task) => !task.enabled)
  } else if (filterEnabled.value === 'error') {
    result = result.filter(
      (task) =>
        task.last_result === 'failed' ||
        task.last_result === 'timeout'
    )
  }

  return result
})

const totalTasks = computed(() => filteredTasks.value.length)

const pagedTasks = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredTasks.value.slice(start, end)
})

const currentMissedList = computed(() => {
  const currentTaskNames = new Set(filteredTasks.value.map((t) => t.name))
  const result = []
  Object.keys(missedMap.value).forEach((taskName) => {
    if (currentTaskNames.has(taskName)) {
      missedMap.value[taskName].forEach((item) => {
        if (!item.compensated) {
          result.push(item)
        }
      })
    }
  })
  return result
})

async function loadData() {
  try {
    const params = {}
    if (filterTag.value) {
      params.tag = filterTag.value
    }
    if (filterEnabled.value === 'true' || filterEnabled.value === 'false') {
      params.enabled = filterEnabled.value
    }

    const [taskData, missedData] = await Promise.all([
      api.getTasks(params),
      api.getMissed(),
    ])

    allTasks.value = taskData || []
    tasks.value = taskData || []
    total.value = taskData?.length || 0

    const map = {}
    ;(missedData || []).forEach((item) => {
      if (!map[item.task_name]) {
        map[item.task_name] = []
      }
      map[item.task_name].push(item)
    })
    missedMap.value = map
  } catch (err) {
    ElMessage.error(err.message || '加载数据失败')
  }
}

function formatTime(t) {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

function getResultTag(status) {
  switch (status) {
    case 'success':
      return { type: 'success', text: '成功' }
    case 'failed':
      return { type: 'danger', text: '失败' }
    case 'running':
      return { type: 'primary', text: '运行中' }
    case 'timeout':
      return { type: 'warning', text: '超时' }
    case 'pending':
      return { type: 'info', text: '等待中' }
    case 'skipped':
      return { type: 'info', text: '跳过' }
    case 'interrupted':
      return { type: 'warning', text: '中断' }
    default:
      return { type: 'info', text: status || '-' }
  }
}

function getStatusTag(task) {
  if (task.last_result === 'running') {
    return { type: 'primary', text: '运行中' }
  }
  if (
    task.last_result === 'failed' ||
    task.last_result === 'timeout'
  ) {
    return { type: 'danger', text: '异常' }
  }
  if (task.enabled) {
    return { type: 'success', text: '启用' }
  }
  return { type: 'info', text: '暂停' }
}

async function handleToggleEnabled(task) {
  try {
    if (task.enabled) {
      await api.disableTask(task.name)
      ElMessage.success('已暂停任务')
    } else {
      await api.enableTask(task.name)
      ElMessage.success('已启用任务')
    }
    loadData()
  } catch (err) {
    ElMessage.error(err.message || '操作失败')
  }
}

async function handleTrigger(task) {
  try {
    await api.triggerTask(task.name)
    ElMessage.success('已触发')
  } catch (err) {
    ElMessage.error(err.message || '触发失败')
  }
}

async function handleDelete(task) {
  try {
    await api.deleteTask(task.name)
    ElMessage.success('删除成功')
    loadData()
  } catch (err) {
    ElMessage.error(err.message || '删除失败')
  }
}

function handlePageChange(page) {
  currentPage.value = page
}

function handleSizeChange(size) {
  pageSize.value = size
  currentPage.value = 1
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.task-list {
  padding: 20px;
}

.toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>
