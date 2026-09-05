<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { serviceStatus, initServiceListener } from './stores/service'

const route = useRoute()

const nav = [
  { to: '/', label: '概览' },
  { to: '/config', label: '配置' },
  { to: '/service', label: '服务' },
  { to: '/users', label: '用户' },
  { to: '/comments', label: '评论' },
  { to: '/logs', label: '日志' },
]

let cleanup: (() => void) | undefined

onMounted(() => {
  cleanup = initServiceListener()
})

onUnmounted(() => {
  cleanup?.()
})

function statusLabel(s: string): string {
  const map: Record<string, string> = {
    running: '运行中',
    stopped: '已停止',
    starting: '启动中',
    stopping: '停止中',
    error: '异常',
    unknown: '未知',
  }
  return map[s] || s
}
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="brand">ebook-server</div>
      <nav>
        <router-link v-for="n in nav" :key="n.to" :to="n.to" active-class="active">{{ n.label }}</router-link>
      </nav>
      <div class="status-bar">
        <span class="status-dot" :class="serviceStatus"></span>
        {{ statusLabel(serviceStatus) }}
      </div>
    </aside>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>
