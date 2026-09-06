<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  runtimeInfo,
  serviceStatus,
  statusLabel,
  refreshRuntimeInfo,
} from '../stores/service'
import { fetchStats, type Stats } from '../api'
import * as ipc from '../electron'
import { baseName, formatBytes, formatUptime } from '../format'

const stats = ref<Stats | null>(null)
const statsError = ref('')
let pollTimer: ReturnType<typeof setInterval> | null = null

async function loadStats(): Promise<void> {
  statsError.value = ''
  try {
    const resp = await fetchStats()
    if (resp.code === '00000' && resp.data) {
      stats.value = resp.data
    } else {
      stats.value = null
      statsError.value = resp.error || '无法读取统计数据'
    }
  } catch {
    stats.value = null
    statsError.value = '后端服务未启动'
  }
}

onMounted(async () => {
  await loadStats()
  void refreshRuntimeInfo()
  // 运行时间要走动，数据库大小要跟着写评论变，所以按秒取一次主进程快照
  pollTimer = setInterval(() => { void refreshRuntimeInfo() }, 1000)
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})

const endpoints = computed(() => runtimeInfo.value?.endpoints ?? null)

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
        <div class="label">服务状态：{{ statusLabel(serviceStatus) }}</div>
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

    <p v-if="statsError" class="err">{{ statsError }}</p>

    <div class="card">
      <h3 style="font-size: 16px; margin-bottom: 12px;">运行信息</h3>
      <dl style="display: grid; grid-template-columns: 130px 1fr; gap: 6px 12px; margin: 0;">
        <div><dt style="color: #64748b;">公开 API</dt><dd style="margin: 0;">{{ endpoints?.apiBaseUrl ?? '-' }}</dd></div>
        <div><dt style="color: #64748b;">管理后台</dt><dd style="margin: 0;">{{ endpoints?.adminOrigin ?? '-' }}</dd></div>
        <div><dt style="color: #64748b;">数据库</dt><dd style="margin: 0;">
          {{ endpoints?.dbPath ? `${baseName(endpoints.dbPath)} (${runtimeInfo?.dbExists ? formatBytes(runtimeInfo.dbSizeBytes) : '尚未创建'})` : '-' }}
        </dd></div>
        <div><dt style="color: #64748b;">运行时间</dt><dd style="margin: 0;">
          {{ serviceStatus === 'running' ? formatUptime(runtimeInfo?.uptimeMs ?? 0) : '—' }}
        </dd></div>
      </dl>
      <p v-if="runtimeInfo?.configStale" class="sub" style="margin-top: 12px;">
        配置已保存但尚未生效，需重启服务。
      </p>
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
