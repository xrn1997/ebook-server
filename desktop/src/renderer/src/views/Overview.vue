<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { serviceStatus } from '../stores/service'
import { fetchStats } from '../api'
import * as ipc from '../electron'

const stats = ref<{ users: number; comments: number } | null>(null)
const statsError = ref('')

onMounted(async () => {
  try {
    const resp = await fetchStats()
    if (resp.code === '00000') {
      stats.value = resp.data
    } else {
      statsError.value = '无法连接后端服务'
    }
  } catch {
    statsError.value = '后端服务未启动'
  }
})

async function handleRestart() {
  await ipc.restartService()
}

async function handleStop() {
  await ipc.stopService()
}
</script>

<template>
  <div>
    <h2>概览</h2>
    <p class="sub">服务状态与基础数据统计</p>

    <div class="grid" style="margin-bottom: 24px;">
      <div class="stat-card">
        <div class="num" :style="{ color: serviceStatus === 'running' ? '#16a34a' : serviceStatus === 'error' ? '#dc2626' : '#64748b' }">
          {{ serviceStatus === 'running' ? '●' : serviceStatus === 'error' ? '✕' : '○' }}
        </div>
        <div class="label">服务状态：{{ serviceStatus }}</div>
      </div>
      <div class="stat-card">
        <div class="num">{{ stats?.users ?? '-' }}</div>
        <div class="label">注册用户</div>
      </div>
      <div class="stat-card">
        <div class="num">{{ stats?.comments ?? '-' }}</div>
        <div class="label">评论总数</div>
      </div>
    </div>

    <div class="card">
      <h3 style="font-size: 16px; margin-bottom: 12px;">快捷操作</h3>
      <div style="display: flex; gap: 8px;">
        <button class="btn btn-primary" @click="handleRestart" :disabled="serviceStatus === 'starting'">
          重启服务
        </button>
        <button class="btn btn-danger" @click="handleStop" :disabled="serviceStatus === 'stopped' || serviceStatus === 'stopping'">
          停止服务
        </button>
      </div>
    </div>
  </div>
</template>
