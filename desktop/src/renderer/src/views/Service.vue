<script setup lang="ts">
import { ref, computed, onMounted, nextTick, watch } from 'vue'
import { serviceStatus, serviceLogs, serviceError, statusLabel } from '../stores/service'
import * as ipc from '../electron'
import type { PortCheckResult } from '../../../shared/types'

const logContainer = ref<HTMLElement | null>(null)

onMounted(async () => {
  serviceLogs.value = await ipc.getLogs()
  await nextTick()
  scrollToEnd()
})

function scrollToEnd() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

// 新日志到达时跟随到底部，否则实时日志会一直停在打开时的位置
watch(
  () => serviceLogs.value.length,
  () => { nextTick(scrollToEnd) },
)

async function handleStart() {
  await ipc.startService()
}

async function handleStop() {
  await ipc.stopService()
}

async function handleRestart() {
  await ipc.restartService()
}

function dismissError() {
  serviceError.value = null
}

const portCheck = ref<PortCheckResult | null>(null)
const checkingPort = ref(false)

async function checkPortNow() {
  checkingPort.value = true
  try {
    portCheck.value = await ipc.checkPort()
  } finally {
    checkingPort.value = false
  }
}

const portCheckText = computed(() => {
  const c = portCheck.value
  if (!c || c.port === 0) return ''
  if (c.inUse && serviceStatus.value === 'running') return `端口 ${c.port} 正由本服务使用`
  if (c.inUse) return `端口 ${c.port} 已被其他进程占用，启动会失败——请在配置页改端口`
  return `端口 ${c.port} 空闲，可以启动`
})
</script>

<template>
  <div>
    <h2>服务</h2>
    <p class="sub">管理 Go 后端服务进程</p>

    <div class="card">
      <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 16px;">
        <span class="status-dot" :class="serviceStatus" style="width: 12px; height: 12px;"></span>
        <strong>当前状态：{{ statusLabel(serviceStatus) }}</strong>
      </div>
      <div v-if="serviceError" class="alert alert-error">
        {{ serviceError.message }}
        <a href="#" style="margin-left: 8px; text-decoration: underline;" @click.prevent="dismissError">忽略</a>
      </div>
      <div style="display: flex; gap: 8px; margin-top: 12px;">
        <!-- stopping 期间禁用启动：旧进程还在宽限期、端口未释放，此刻启动只会得到假的「端口被占用」 -->
        <button class="btn btn-success" @click="handleStart" :disabled="serviceStatus === 'running' || serviceStatus === 'starting' || serviceStatus === 'stopping'">
          启动
        </button>
        <button class="btn btn-danger" @click="handleStop" :disabled="serviceStatus === 'stopped' || serviceStatus === 'stopping'">
          停止
        </button>
        <button class="btn btn-primary" @click="handleRestart" :disabled="serviceStatus === 'starting'">
          重启
        </button>
      </div>
      <div style="margin-top: 8px; display: flex; gap: 8px; align-items: center;">
        <button class="btn" @click="checkPortNow" :disabled="checkingPort">
          {{ checkingPort ? '检测中...' : '检测端口' }}
        </button>
        <span v-if="portCheckText" class="sub">{{ portCheckText }}</span>
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
