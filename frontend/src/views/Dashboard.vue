<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { fetchStats, unwrap, type Stats } from '../api'

const stats = ref<Stats | null>(null)
const error = ref('')

// 只在 mounted 取一次，不另立 loading 标志：stats 为 null 就是模板里的「加载中…」。
// 就这一处取位，直接经 unwrap 即可，不必上 useAdminDetail
onMounted(async () => {
  const result = await unwrap(fetchStats(), '加载失败')
  if (result.ok && result.data !== undefined) stats.value = result.data
  // 成功却没带 data 也是契约漂移：让它落进 else，否则模板永远停在「加载中…」，
  // 把后端改动藏成一次看似正常的挂起。
  else error.value = result.ok ? '统计数据格式不正确' : result.message
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