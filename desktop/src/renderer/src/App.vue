<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { serviceStatus, serviceError, initServiceListener, statusLabel } from './stores/service'

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
      <!-- 主进程才知道失败的确切原因（端口被占用、配置无效、自动登录失败），所以由它报上来统一提示 -->
      <div v-if="serviceError" class="alert alert-error" style="margin-bottom: 16px;">
        {{ serviceError.message }}
      </div>
      <router-view />
    </main>
  </div>
</template>
