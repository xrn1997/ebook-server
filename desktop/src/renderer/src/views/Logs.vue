<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchLogs } from '../api'

const rows = ref<any[]>([])
const error = ref('')
const openId = ref<number | null>(null)

onMounted(async () => {
  try {
    const resp = await fetchLogs()
    if (resp.code === '00000') rows.value = resp.data.list
    else error.value = resp.error || '加载失败'
  } catch {
    error.value = '无法连接后端服务'
  }
})

function toggle(id: number) {
  openId.value = openId.value === id ? null : id
}
</script>

<template>
  <div>
    <h2>操作日志</h2>
    <p class="sub">后台审计视图：点击行可展开详情</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr>
          <th>时间</th>
          <th>方法</th>
          <th>路径</th>
          <th>IP</th>
          <th>业务码</th>
        </tr>
      </thead>
      <tbody>
        <template v-for="l in rows" :key="l.id">
          <tr style="cursor: pointer;" @click="toggle(l.id)" @mouseover="($event.currentTarget as HTMLElement).style.background='#f8fafc'" @mouseout="($event.currentTarget as HTMLElement).style.background=''">
            <td>{{ l.created_at }}</td>
            <td><code>{{ l.method }}</code></td>
            <td style="max-width: 300px; overflow: hidden; text-overflow: ellipsis;">{{ l.path }}</td>
            <td>{{ l.ip }}</td>
            <td>
              <span class="badge" :class="l.error_code && l.error_code !== '00000' ? 'err' : 'ok'">
                {{ l.error_code || '—' }}
              </span>
            </td>
          </tr>
          <tr v-if="openId === l.id">
            <td colspan="5" style="background: #f8fafc;">
              <dl style="display: grid; grid-template-columns: 110px 1fr; gap: 6px 12px; margin: 0;">
                <div><dt style="color: #64748b;">ID</dt><dd style="margin: 0;">{{ l.id }}</dd></div>
                <div><dt style="color: #64748b;">用户</dt><dd style="margin: 0;">{{ l.user_id || l.username || '-' }}</dd></div>
                <div><dt style="color: #64748b;">状态码</dt><dd style="margin: 0;">{{ l.response_code }}</dd></div>
                <div><dt style="color: #64748b;">业务码</dt><dd style="margin: 0;">{{ l.error_code || '—' }}</dd></div>
                <div><dt style="color: #64748b;">业务文案</dt><dd style="margin: 0;">{{ l.error_message || '—' }}</dd></div>
                <div><dt style="color: #64748b;">User-Agent</dt><dd style="margin: 0; word-break: break-all;">{{ l.user_agent || '—' }}</dd></div>
              </dl>
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>
