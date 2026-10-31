<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAdminList } from '../useAdminList'
import { fetchLogs, SUCCESS_CODE, type AdminLog } from '../api'
import Pager from '../components/Pager.vue'

const method = ref('')
const path = ref('')
const onlyFailed = ref(false)
const openId = ref<number | null>(null)

const { rows, total, page, pageCount, hasPrev, hasNext, error, loading, reload, nextPage, prevPage } =
  useAdminList<AdminLog>((p) =>
    fetchLogs({ ...p, method: method.value, path: path.value, failed: onlyFailed.value }),
  )

onMounted(reload)

/** 筛选条件变化后回到第 1 页 */
function search(): void {
  void reload()
}

/** 展开/收起一条日志的详情（同一时刻只展开一行） */
function toggle(id: number): void {
  openId.value = openId.value === id ? null : id
}

/**
 * 业务码徽标的样式。
 *
 * 空码不是成功：审计只在响应体是 JSON 时才取得到信封，空值意味着「没观察到成败」
 * （非 JSON / 超过 4 KiB 的成功响应被截断），按成功渲染会把审计盲区说成正常。
 */
function codeClass(code: string): string {
  if (!code) return 'unknown'
  return code === SUCCESS_CODE ? 'ok' : 'err'
}
</script>

<template>
  <div>
    <h2>操作日志</h2>
    <p class="sub">后台审计视图：点击行可展开详情</p>

    <div class="card toolbar">
      <select v-model="method" class="method-select" @change="search">
        <option value="">全部方法</option>
        <option value="GET">GET</option>
        <option value="POST">POST</option>
        <option value="PUT">PUT</option>
        <option value="DELETE">DELETE</option>
      </select>
      <input v-model="path" placeholder="路径片段" @keyup.enter="search" />
      <label class="check">
        <input v-model="onlyFailed" type="checkbox" @change="search" /> 只看失败
      </label>
      <button class="btn btn-primary" :disabled="loading" @click="search">筛选</button>
    </div>

    <p v-if="loading" class="sub">加载中…</p>
    <p v-else-if="error" class="err">{{ error }}</p>
    <template v-else>
      <table>
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
            <tr class="row" @click="toggle(l.id)">
              <td>{{ l.created_at }}</td>
              <td><code>{{ l.method }}</code></td>
              <td class="path">{{ l.path }}</td>
              <td>{{ l.ip }}</td>
              <td>
                <span class="badge" :class="codeClass(l.error_code)">
                  {{ l.error_code || '—' }}
                </span>
              </td>
            </tr>
            <tr v-if="openId === l.id">
              <td colspan="5" class="detail-cell">
                <dl class="detail-grid">
                  <div><dt>ID</dt><dd>{{ l.id }}</dd></div>
                  <div><dt>用户</dt><dd>{{ l.username || l.user_id || '-' }}</dd></div>
                  <div><dt>状态码</dt><dd>{{ l.response_code }}</dd></div>
                  <div><dt>业务码</dt><dd>{{ l.error_code || '—' }}</dd></div>
                  <div><dt>业务文案</dt><dd>{{ l.error_message || '—' }}</dd></div>
                  <div><dt>User-Agent</dt><dd class="wrap">{{ l.user_agent || '—' }}</dd></div>
                </dl>
              </td>
            </tr>
          </template>
          <tr v-if="rows.length === 0">
            <td colspan="5" class="sub">没有匹配的记录</td>
          </tr>
        </tbody>
      </table>

      <Pager
        :page="page"
        :page-count="pageCount"
        :total="total"
        unit="条记录"
        :has-prev="hasPrev"
        :has-next="hasNext"
        @prev="prevPage"
        @next="nextPage"
      />
    </template>
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 8px; align-items: center; }
.toolbar input:not([type="checkbox"]) { flex: 1; padding: 8px 12px; border: 1px solid #cbd5e1; border-radius: 8px; }
.method-select { width: 120px; padding: 8px 12px; border: 1px solid #cbd5e1; border-radius: 8px; }
.check { display: flex; align-items: center; gap: 4px; font-size: 14px; }
.row { cursor: pointer; }
.row:hover { background: #f8fafc; }
.path { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.detail-cell { background: #f8fafc; }
.detail-grid { display: grid; grid-template-columns: 110px 1fr; gap: 6px 12px; margin: 0; }
.detail-grid dt { color: #64748b; }
.detail-grid dd { margin: 0; }
.wrap { word-break: break-all; }
</style>
