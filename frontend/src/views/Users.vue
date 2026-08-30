<script setup>
import { onMounted, ref } from 'vue'
import { fetchUsers } from '../api.js'

const rows = ref([])
const error = ref('')

onMounted(async () => {
  const resp = await fetchUsers()
  if (resp.code === '00000') rows.value = resp.data.list
  else error.value = resp.error || '加载失败'
})
</script>

<template>
  <div>
    <h2>用户管理</h2>
    <p class="sub">账号列表（只读；封禁/详情待扩展）。</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr><th>UID</th><th>邮箱</th><th>展示名</th><th>昵称</th><th>注册时间</th></tr>
      </thead>
      <tbody>
        <tr v-for="u in rows" :key="u.uid">
          <td>{{ u.uid }}</td>
          <td>{{ u.email }}</td>
          <td>{{ u.username }}</td>
          <td>{{ u.nickname }}</td>
          <td>{{ u.created_at }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
h2 { font-size: 20px; }
.sub { color: #64748b; font-size: 14px; }
table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #e2e8f0; border-radius: 12px; overflow: hidden; }
th, td { text-align: left; padding: 10px 14px; border-bottom: 1px solid #e2e8f0; font-size: 14px; }
th { background: #f8fafc; color: #475569; }
.err { color: #dc2626; }
</style>