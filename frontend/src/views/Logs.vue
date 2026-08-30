<script setup>
import { onMounted, ref } from 'vue'
import { fetchLogs } from '../api.js'

const rows = ref([])
const error = ref('')

onMounted(async () => {
  const resp = await fetchLogs()
  if (resp.code === '00000') rows.value = resp.data.list
  else error.value = resp.error || '加载失败'
})
</script>

<template>
  <div>
    <h2>请求日志</h2>
    <p class="sub">后台审计视图：每次请求都会写入 operation_logs（仅元信息，不含请求体）。</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr><th>ID</th><th>时间</th><th>用户</th><th>方法</th><th>路径</th><th>IP</th><th>状态</th></tr>
      </thead>
      <tbody>
        <tr v-for="l in rows" :key="l.id">
          <td>{{ l.id }}</td>
          <td>{{ l.created_at }}</td>
          <td>{{ l.user_id || l.username || '-' }}</td>
          <td><code>{{ l.method }}</code></td>
          <td class="path">{{ l.path }}</td>
          <td>{{ l.ip }}</td>
          <td>{{ l.response_code }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
h2 { font-size: 20px; }
.sub { color: #64748b; font-size: 14px; }
table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #e2e8f0; border-radius: 12px; overflow: hidden; }
th, td { text-align: left; padding: 8px 12px; border-bottom: 1px solid #e2e8f0; font-size: 13px; white-space: nowrap; }
.path { max-width: 320px; overflow: hidden; text-overflow: ellipsis; }
th { background: #f8fafc; color: #475569; }
.err { color: #dc2626; }
</style>