<script setup>
import { useRoute } from 'vue-router'
import { token } from './api.js'

const route = useRoute()
const nav = [
  { to: '/', label: '概览 / 统计' },
  { to: '/users', label: '用户管理' },
  { to: '/comments', label: '内容审核' },
  { to: '/logs', label: '请求日志' },
]
</script>

<template>
  <div v-if="route.name === 'login'" class="full">
    <router-view />
  </div>
  <div v-else class="layout">
    <aside class="side">
      <div class="brand">ebook 后台</div>
      <nav>
        <router-link v-for="n in nav" :key="n.to" :to="n.to" active-class="active">{{ n.label }}</router-link>
      </nav>
      <button class="logout" @click="token.set(''); location.hash = '#/login'">退出登录</button>
    </aside>
    <main class="main">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.full { min-height: 100vh; display: flex; align-items: center; justify-content: center; }
.layout { display: flex; min-height: 100vh; }
.side { width: 200px; background: #0f172a; color: #e2e8f0; padding: 16px; display: flex; flex-direction: column; gap: 8px; }
.brand { font-weight: 700; font-size: 18px; padding: 8px 4px 16px; }
.side nav { display: flex; flex-direction: column; gap: 4px; }
.side nav a { padding: 10px 12px; border-radius: 8px; }
.side nav a:hover { background: #1e293b; }
.side nav a.active { background: #334155; font-weight: 600; }
.logout { margin-top: auto; background: none; border: none; color: #94a3b8; text-align: left; padding: 10px 12px; }
.logout:hover { color: #fca5a5; }
.main { flex: 1; padding: 24px; }
</style>