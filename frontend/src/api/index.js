import axios from 'axios'
import { ElMessage } from 'element-plus'

const request = axios.create({
  baseURL: '/api',
  timeout: 30000
})

request.interceptors.response.use(
  (response) => {
    const { success, data, message } = response.data
    if (success) {
      return data
    } else {
      ElMessage.error(message || '请求失败')
      return Promise.reject(new Error(message || '请求失败'))
    }
  },
  (error) => {
    ElMessage.error(error.message || '网络错误')
    return Promise.reject(error)
  }
)

export const getTasks = (params) => {
  return request.get('/tasks', { params })
}

export const getTask = (name) => {
  return request.get(`/tasks/${name}`)
}

export const createTask = (data) => {
  return request.post('/tasks', data)
}

export const updateTask = (name, data) => {
  return request.put(`/tasks/${name}`, data)
}

export const deleteTask = (name) => {
  return request.delete(`/tasks/${name}`)
}

export const triggerTask = (name) => {
  return request.post(`/tasks/${name}/trigger`)
}

export const enableTask = (name) => {
  return request.post(`/tasks/${name}/enable`)
}

export const disableTask = (name) => {
  return request.post(`/tasks/${name}/disable`)
}

export const previewCron = (expr, count = 5) => {
  return request.post('/cron/preview', { expr, count })
}

export const getDAG = () => {
  return request.get('/dag')
}

export const getExecutions = (params) => {
  return request.get('/executions', { params })
}

export const getExecution = (id) => {
  return request.get(`/executions/${id}`)
}

export const getAlerts = (params) => {
  return request.get('/alerts', { params })
}

export const getSettings = () => {
  return request.get('/settings')
}

export const updateSettings = (data) => {
  return request.post('/settings', data)
}

export const getMissed = (params) => {
  return request.get('/missed', { params })
}

export const detectMissed = () => {
  return request.post('/missed/detect')
}

export const getHealth = () => {
  return request.get('/health')
}

export const batchEnableTasks = (taskNames) => {
  return request.post('/tasks/batch/enable', { task_names: taskNames })
}

export const batchDisableTasks = (taskNames) => {
  return request.post('/tasks/batch/disable', { task_names: taskNames })
}

export const batchDeleteTasks = (taskNames) => {
  return request.post('/tasks/batch/delete', { task_names: taskNames })
}

export const testWebhook = (webhookUrl) => {
  return request.post('/settings/webhook/test', { webhook_url: webhookUrl })
}

export const getRunningExecutions = () => {
  return request.get('/executions/running')
}
