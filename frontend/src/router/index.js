import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    redirect: '/tasks'
  },
  {
    path: '/tasks',
    name: 'TaskList',
    component: () => import('../views/TaskList.vue')
  },
  {
    path: '/tasks/new',
    name: 'TaskCreate',
    component: () => import('../views/TaskEdit.vue')
  },
  {
    path: '/tasks/:name/edit',
    name: 'TaskEdit',
    component: () => import('../views/TaskEdit.vue')
  },
  {
    path: '/dag',
    name: 'DAGView',
    component: () => import('../views/DAGView.vue')
  },
  {
    path: '/executions',
    name: 'ExecutionHistory',
    component: () => import('../views/ExecutionHistory.vue')
  },
  {
    path: '/calendar',
    name: 'ScheduleCalendar',
    component: () => import('../views/ScheduleCalendar.vue')
  },
  {
    path: '/monitor',
    name: 'RealtimeMonitor',
    component: () => import('../views/RealtimeMonitor.vue')
  },
  {
    path: '/settings',
    name: 'SystemSettings',
    component: () => import('../views/SystemSettings.vue')
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

export default router
