<template>
  <el-container class="layout-container">
    <el-header class="layout-header">
      <h2 class="header-title">Cron调度管理系统</h2>
    </el-header>
    <el-container>
      <el-aside width="200px" class="layout-aside">
        <el-menu
          :default-active="activeMenu"
          router
          background-color="#f5f7fa"
          text-color="#333"
          active-text-color="#409eff"
        >
          <el-menu-item index="/tasks">
            <el-icon><List /></el-icon>
            <span>任务列表</span>
          </el-menu-item>
          <el-menu-item index="/dag">
            <el-icon><Share /></el-icon>
            <span>DAG视图</span>
          </el-menu-item>
          <el-menu-item index="/executions">
            <el-icon><Timer /></el-icon>
            <span>执行历史</span>
          </el-menu-item>
          <el-menu-item index="/settings">
            <el-icon><Setting /></el-icon>
            <span>系统设置</span>
          </el-menu-item>
        </el-menu>
      </el-aside>
      <el-main class="layout-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const activeMenu = computed(() => {
  if (route.path.startsWith('/tasks')) {
    return '/tasks'
  }
  return route.path
})
</script>

<style scoped>
.layout-container {
  height: 100vh;
}

.layout-header {
  background-color: #409eff;
  color: #fff;
  display: flex;
  align-items: center;
  padding: 0 20px;
}

.header-title {
  margin: 0;
  color: #fff;
  font-size: 20px;
}

.layout-aside {
  background-color: #f5f7fa;
  border-right: 1px solid #e4e7ed;
}

.layout-main {
  background-color: #fff;
  padding: 20px;
}

.el-menu {
  border-right: none;
}
</style>
