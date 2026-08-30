<script setup>
import { onMounted, ref } from 'vue'
import { fetchComments } from '../api.js'

const rows = ref([])
const error = ref('')

onMounted(async () => {
  const resp = await fetchComments()
  if (resp.code === '00000') rows.value = resp.data.list
  else error.value = resp.error || '加载失败'
})
</script>

<template>
  <div>
    <h2>内容审核</h2>
    <p class="sub">评论列表（只读；删除/拉黑待扩展）。</p>
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

<style scoped>
h2 { font-size: 20px; }
.sub { color: #64748b; font-size: 14px; }
table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #e2e8f0; border-radius: 12px; overflow: hidden; }
th, td { text-align: left; padding: 10px 14px; border-bottom: 1px solid #e2e8f0; font-size: 14px; }
th { background: #f8fafc; color: #475569; }
.err { color: #dc2626; }
</style>