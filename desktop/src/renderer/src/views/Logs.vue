<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAdminList } from '../useAdminList'
import { fetchLogs, type AdminLog } from '../api'
import Pager from '../components/Pager.vue'

const method = ref('')
const path = ref('')
const onlyFailed = ref(false)
const openId = ref<number | null>(null)

const { rows, total, page, pageCount, hasPrev, hasNext, error, reload, nextPage, prevPage } =
  useAdminList<AdminLog>((p) =>
    fetchLogs({ ...p, method: method.value, path: path.value, failed: onlyFailed.value }),
  )

onMounted(reload)

/** 筛选条件变化后回到第 1 页 */
function search() {
  void reload()
}

/** 展开/收起一条日志的详情（同一时刻只展开一行） */
function toggle(id: number) {
  openId.value = openId.value === id ? null : id
}
</script>

<template>
  <div>
    <h2>操作日志</h2>
    <p class="sub">后台审计视图：点击行可展开详情</p>

    <div class="card" style="display: flex; gap: 8px; align-items: center;">
      <select v-model="method" style="width: 120px;" @change="search">
        <option value="">全部方法</option>
        <option value="GET">GET</option>
        <option value="POST">POST</option>
        <option value="PUT">PUT</option>
        <option value="DELETE">DELETE</option>
      </select>
      <input v-model="path" placeholder="路径片段" style="flex: 1;" @keyup.enter="search" />
      <label>
        <input type="checkbox" v-model="onlyFailed" @change="search" /> 只看失败
      </label>
      <button class="btn btn-primary" @click="search">筛选</button>
    </div>

    <p v-if="error" class="err">{{ error }}</p>
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
