<script setup>
import { onMounted, ref } from 'vue'
import { fetchLogs } from '../api.js'

const rows = ref([])
const error = ref('')
const openId = ref(null)

onMounted(async () => {
  const resp = await fetchLogs()
  if (resp.code === '00000') rows.value = resp.data.list
  else error.value = resp.error || '加载失败'
})

function toggle(id) {
  openId.value = openId.value === id ? null : id
}
</script>

<template>
  <div>
    <h2>请求日志</h2>
    <p class="sub">后台审计视图：点击行可展开详情。error_code 为 "00000" 表示成功，非 00000 即业务失败。</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr>
          <th>时间</th>
          <th>方法</th>
          <th>路径</th>
          <th>IP</th>
          <th class="code-col">业务码</th>
        </tr>
      </thead>
      <tbody>
        <template v-for="l in rows" :key="l.id">
          <tr class="row" @click="toggle(l.id)">
            <td>{{ l.created_at }}</td>
            <td><code>{{ l.method }}</code></td>
            <td class="path">{{ l.path }}</td>
            <td>{{ l.ip }}</td>
            <td>
              <span :class="['badge', l.error_code && l.error_code !== '00000' ? 'err' : 'ok']">
                {{ l.error_code || '—' }}
              </span>
            </td>
          </tr>
          <tr v-if="openId === l.id" class="detail-row">
            <td colspan="5">
              <dl>
                <div><dt>ID</dt><dd>{{ l.id }}</dd></div>
                <div><dt>用户</dt><dd>{{ l.user_id || l.username || '-' }}</dd></div>
                <div><dt>状态</dt><dd>{{ l.response_code }}</dd></div>
                <div><dt>业务码</dt><dd>{{ l.error_code || '—' }}</dd></div>
                <div><dt>业务文案</dt><dd>{{ l.error_message || '—' }}</dd></div>
                <div><dt>User-Agent</dt><dd class="wrap">{{ l.user_agent || '—' }}</dd></div>
                <div><dt>发起时间</dt><dd>{{ l.created_at }}</dd></div>
              </dl>
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
h2 { font-size: 20px; }
.sub { color: #64748b; font-size: 14px; }
table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #e2e8f0; border-radius: 12px; overflow: hidden; }
th, td { text-align: left; padding: 8px 12px; border-bottom: 1px solid #e2e8f0; font-size: 13px; white-space: nowrap; }
th { background: #f8fafc; color: #475569; }
.path { max-width: 300px; overflow: hidden; text-overflow: ellipsis; }
.row { cursor: pointer; }
.row:hover { background: #f8fafc; }
.badge { padding: 1px 8px; border-radius: 9999px; font-size: 12px; font-weight: 600; }
.badge.ok { background: #ecfdf5; color: #047857; }
.badge.err { background: #fef2f2; color: #b91c1c; }
.detail-row td { background: #f8fafc; white-space: normal; }
dl { margin: 0; display: grid; grid-template-columns: 110px 1fr; gap: 6px 12px; }
dt { color: #64748b; }
dd { margin: 0; word-break: break-all; }
.err { color: #dc2626; }
</style>