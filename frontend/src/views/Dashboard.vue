<script setup>
import { onMounted, ref } from 'vue'
import { fetchStats } from '../api.js'

const stats = ref(null)
const error = ref('')

onMounted(async () => {
  const resp = await fetchStats()
  if (resp.code === '00000') stats.value = resp.data
  else error.value = resp.error || '加载失败'
})
</script>

<template>
  <div>
    <h2>概览 / 统计</h2>
    <p class="sub">只读运维概览与基础数据统计（图表待扩展）。</p>
    <div v-if="error" class="err">{{ error }}</div>
    <div v-else-if="stats" class="grid">
      <div class="stat"><div class="num">{{ stats.users }}</div><div class="label">注册用户</div></div>
      <div class="stat"><div class="num">{{ stats.comments }}</div><div class="label">评论总数</div></div>
    </div>
    <p class="sub dim" v-else>加载中…</p>
  </div>
</template>

<style scoped>
h2 { font-size: 20px; }
.sub { color: #64748b; font-size: 14px; }
.dim { color: #94a3b8; }
.grid { display: flex; gap: 16px; margin-top: 16px; }
.stat { background: #fff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 20px 28px; min-width: 140px; }
.num { font-size: 28px; font-weight: 700; }
.label { color: #64748b; font-size: 13px; margin-top: 4px; }
.err { color: #dc2626; }
</style>