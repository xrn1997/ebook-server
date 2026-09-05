<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchComments } from '../api'

const rows = ref<any[]>([])
const error = ref('')

onMounted(async () => {
  try {
    const resp = await fetchComments()
    if (resp.code === '00000') rows.value = resp.data.list
    else error.value = resp.error || '加载失败'
  } catch {
    error.value = '无法连接后端服务'
  }
})
</script>

<template>
  <div>
    <h2>评论管理</h2>
    <p class="sub">评论列表</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr><th>ID</th><th>作者</th><th>内容</th><th>时间</th></tr>
      </thead>
      <tbody>
        <tr v-for="c in rows" :key="c.id">
          <td>{{ c.id }}</td>
          <td>{{ c.user?.username || c.user_id }}</td>
          <td>{{ c.content }}</td>
          <td>{{ c.created_at }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
