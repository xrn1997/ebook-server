<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { serviceStatus, serviceLogs } from '../stores/service'
import * as ipc from '../electron'

const logContainer = ref<HTMLElement | null>(null)
let cleanup: (() => void) | undefined

onMounted(async () => {
  const lines = await ipc.getLogs()
  serviceLogs.value = lines
  await nextTick()
  scrollToEnd()
})

onUnmounted(() => {
  cleanup?.()
})

function scrollToEnd() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

async function handleStart() {
  await ipc.startService()
}

async function handleStop() {
  await ipc.stopService()
}

async function handleRestart() {
  await ipc.restartService()
}
</script>

<template>
  <div>
    <h2>服务</h2>
    <p class="sub">管理 Go 后端服务进程</p>

    <div class="card">
      <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 16px;">
        <span class="status-dot" :class="serviceStatus" style="width: 12px; height: 12px;"></span>
        <strong>当前状态：{{ serviceStatus }}</strong>
      </div>
      <div style="display: flex; gap: 8px;">
        <button class="btn btn-success" @click="handleStart" :disabled="serviceStatus === 'running' || serviceStatus === 'starting'">
          启动
        </button>
        <button class="btn btn-danger" @click="handleStop" :disabled="serviceStatus === 'stopped' || serviceStatus === 'stopping'">
          停止
        </button>
        <button class="btn btn-primary" @click="handleRestart" :disabled="serviceStatus === 'starting'">
          重启
        </button>
      </div>
    </div>

    <div class="card">
      <h3 style="font-size: 16px; margin-bottom: 12px;">实时日志</h3>
      <div ref="logContainer" class="log-viewer">{{ serviceLogs.join('\n') || '（暂无日志）' }}</div>
    </div>
  </div>
</template>

<style scoped>
.status-dot {
  display: inline-block;
  border-radius: 50%;
  background: #94a3b8;
}
.status-dot.running { background: #22c55e; }
.status-dot.error { background: #ef4444; }
.status-dot.starting, .status-dot.stopping { background: #f59e0b; }
</style>
