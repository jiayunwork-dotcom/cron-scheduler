<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>DAG依赖视图</span>
        <div class="toolbar">
          <el-button type="primary" @click="refresh">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
          <el-button @click="autoLayout">
            <el-icon><MagicStick /></el-icon>
            自动布局
          </el-button>
        </div>
      </div>
    </template>

    <div ref="networkContainer" class="network-container"></div>

    <div class="legend">
      <div class="legend-item">
        <span class="legend-color" style="background: #10b981"></span>
        <span>成功</span>
      </div>
      <div class="legend-item">
        <span class="legend-color" style="background: #ef4444"></span>
        <span>失败</span>
      </div>
      <div class="legend-item">
        <span class="legend-color" style="background: #3b82f6"></span>
        <span>运行中</span>
      </div>
      <div class="legend-item">
        <span class="legend-color" style="background: #9ca3af"></span>
        <span>未执行</span>
      </div>
    </div>
  </el-card>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Network, DataSet } from 'vis-network/standalone'
import { Refresh, MagicStick } from '@element-plus/icons-vue'
import * as api from '@/api'

const networkContainer = ref(null)
let networkInstance = null
let nodes = null
let edges = null

const loadDAG = async () => {
  try {
    const data = await api.getDAG()

    const nodeList = data.nodes.map((node) => ({
      id: node.id,
      label: node.name,
      color: {
        background: node.color || '#9ca3af'
      },
      font: { color: '#fff' },
      shape: 'box',
      margin: 10
    }))

    const edgeList = data.edges.map((edge) => ({
      from: edge.source,
      to: edge.target,
      arrows: 'to',
      color: '#999'
    }))

    nodes = new DataSet(nodeList)
    edges = new DataSet(edgeList)

    const options = {
      physics: {
        enabled: true,
        solver: 'forceAtlas2Based'
      },
      interaction: {
        dragNodes: true,
        zoomView: true,
        dragView: true
      },
      layout: {
        improvedLayout: true
      }
    }

    await nextTick()
    if (networkContainer.value) {
      networkInstance = new Network(networkContainer.value, { nodes, edges }, options)
    }
  } catch (error) {
    console.error('加载DAG数据失败:', error)
  }
}

const autoLayout = () => {
  if (networkInstance) {
    networkInstance.setOptions({
      physics: {
        enabled: true,
        solver: 'forceAtlas2Based'
      }
    })
  }
}

const refresh = () => {
  if (networkInstance) {
    networkInstance.destroy()
    networkInstance = null
  }
  loadDAG()
}

onMounted(() => {
  loadDAG()
})

onBeforeUnmount(() => {
  if (networkInstance) {
    networkInstance.destroy()
    networkInstance = null
  }
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.toolbar {
  display: flex;
  gap: 8px;
}

.network-container {
  height: 600px;
  border: 1px solid #eee;
  border-radius: 4px;
}

.legend {
  display: flex;
  gap: 20px;
  margin-top: 16px;
  padding: 12px;
  background: #f9f9f9;
  border-radius: 4px;
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.legend-color {
  display: inline-block;
  width: 16px;
  height: 16px;
  border-radius: 3px;
}
</style>
