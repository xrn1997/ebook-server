<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchUsers } from '../api'

const rows = ref<any[]>([])
const error = ref('')

onMounted(async () => {
  try {
    const resp = await fetchUsers()
    if (resp.code === '00000') rows.value = resp.data.list
    else error.value = resp.error || '加载失败'
  } catch {
    error.value = '无法连接后端服务'
  }
})
</script>

<template>
  <div>
    <h2>用户管理</h2>
    <p class="sub">账号列表</p>
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
