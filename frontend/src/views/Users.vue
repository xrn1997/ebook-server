<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAdminList } from '../useAdminList'
import { fetchUsers, fetchUserDetail, SUCCESS_CODE, type AdminUser, type UserDetail } from '../api'
import Pager from '../components/Pager.vue'

const keyword = ref('')
const detail = ref<UserDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
/** 详情请求序号：连续点开两个账号时，先发后回的旧响应必须整段丢弃。 */
let detailRequestId = 0

const { rows, total, page, pageCount, hasPrev, hasNext, error, loading, reload, nextPage, prevPage } =
  useAdminList<AdminUser>((p) => fetchUsers({ ...p, keyword: keyword.value }))

onMounted(reload)

/** 搜索关键词变化后回到第 1 页；旧列表的详情与错误不再适用 */
function search(): void {
  resetDetail()
  void reload()
}

/**
 * 作废详情区的所有在途/已展示状态。
 *
 * 只清 data 不够：`showDetail` 的序号校验会让旧请求照常落地，把上一个筛选条件下
 * 的账号重新显示出来；翻页后列表里已经没有这个账号了。
 */
function resetDetail(): void {
  detailRequestId++
  detailLoading.value = false
  detail.value = null
  detailError.value = ''
}

/** 翻页同样属于「换了列表」，详情卡片必须一起作废。 */
function changePage(move: () => Promise<void>): void {
  resetDetail()
  void move()
}

/** 展开某账号的详情（同一时刻只看一个） */
async function showDetail(uid: number): Promise<void> {
  const id = ++detailRequestId
  detailLoading.value = true
  detailError.value = ''
  detail.value = null
  const resp = await fetchUserDetail(uid)
  if (id !== detailRequestId) return
  detailLoading.value = false
  if (resp.code === SUCCESS_CODE && resp.data) {
    detail.value = resp.data
  } else {
    detailError.value = resp.error || '加载用户详情失败'
  }
}
</script>

<template>
  <div>
    <h2>用户管理</h2>
    <p class="sub">账号列表：按邮箱 / 用户名 / 昵称搜索，UID 直接命中</p>

    <div class="card toolbar">
      <input v-model="keyword" placeholder="邮箱 / 用户名 / 昵称 / UID" @keyup.enter="search" />
      <button class="btn btn-primary" :disabled="loading" @click="search">搜索</button>
    </div>

    <p v-if="loading" class="sub">加载中…</p>
    <p v-else-if="error" class="err">{{ error }}</p>
    <template v-else>
      <table>
        <thead>
          <tr><th>UID</th><th>邮箱</th><th>用户名</th><th>昵称</th><th>注册时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="u in rows" :key="u.uid">
            <td>{{ u.uid }}</td>
            <td>{{ u.email }}</td>
            <td>{{ u.username }}</td>
            <td>{{ u.nickname || '—' }}</td>
            <td>{{ u.created_at }}</td>
            <td><button type="button" class="link-button" @click="showDetail(u.uid)">详情</button></td>
          </tr>
          <tr v-if="rows.length === 0">
            <td colspan="6" class="sub">没有匹配的账号</td>
          </tr>
        </tbody>
      </table>

      <Pager
        :page="page"
        :page-count="pageCount"
        :total="total"
        unit="个账号"
        :has-prev="hasPrev"
        :has-next="hasNext"
        @prev="changePage(prevPage)"
        @next="changePage(nextPage)"
      />
    </template>

    <div v-if="detailLoading" class="sub">加载详情中...</div>
    <div v-else-if="detailError" class="alert alert-error">{{ detailError }}</div>
    <div v-else-if="detail" class="card">
      <h3 class="detail-title">用户详情：{{ detail.user.username }}</h3>
      <dl class="detail-grid">
        <div><dt>UID</dt><dd>{{ detail.user.uid }}</dd></div>
        <div><dt>邮箱</dt><dd>{{ detail.user.email }}</dd></div>
        <div><dt>昵称</dt><dd>{{ detail.user.nickname || '—' }}</dd></div>
        <div><dt>头像</dt><dd class="wrap">{{ detail.user.avatar || '—' }}</dd></div>
        <div><dt>注册时间</dt><dd>{{ detail.user.created_at }}</dd></div>
        <div><dt>评论数</dt><dd>{{ detail.comment_count }}</dd></div>
      </dl>
    </div>
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 8px; align-items: center; }
.toolbar input { flex: 1; padding: 8px 12px; border: 1px solid #cbd5e1; border-radius: 8px; }
.detail-title { font-size: 16px; margin: 0 0 8px; }
.detail-grid { display: grid; grid-template-columns: 110px 1fr; gap: 6px 12px; margin: 0; }
.detail-grid dt { color: #64748b; }
.detail-grid dd { margin: 0; }
.wrap { word-break: break-all; }
.link-button {
  background: none;
  border: none;
  color: #0f172a;
  padding: 0;
  text-decoration: underline;
}
</style>
