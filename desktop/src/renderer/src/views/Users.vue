<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAdminList } from '../useAdminList'
import { fetchUsers, fetchUserDetail, type AdminUser, type UserDetail } from '../api'
import Pager from '../components/Pager.vue'

const keyword = ref('')
const detail = ref<UserDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')

const { rows, total, page, pageCount, hasPrev, hasNext, error, reload, nextPage, prevPage } =
  useAdminList<AdminUser>((p) => fetchUsers({ ...p, keyword: keyword.value }))

onMounted(reload)

/** 搜索关键词变化后回到第 1 页 */
function search() {
  void reload()
}

/** 展开某账号的详情（同一时刻只看一个） */
async function showDetail(uid: number) {
  detailLoading.value = true
  detailError.value = ''
  detail.value = null
  const resp = await fetchUserDetail(uid)
  detailLoading.value = false
  if (resp.code === '00000' && resp.data) {
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

    <div class="card" style="display: flex; gap: 8px; align-items: center;">
      <input
        v-model="keyword"
        placeholder="邮箱 / 用户名 / 昵称 / UID"
        style="flex: 1;"
        @keyup.enter="search"
      />
      <button class="btn btn-primary" @click="search">搜索</button>
    </div>

    <p v-if="error" class="err">{{ error }}</p>
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
            <td><a href="#" @click.prevent="showDetail(u.uid)">详情</a></td>
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
        @prev="prevPage"
        @next="nextPage"
      />
    </template>

    <div v-if="detailLoading" class="sub">加载详情中...</div>
    <div v-else-if="detailError" class="alert alert-error">{{ detailError }}</div>
    <div v-else-if="detail" class="card">
      <h3 style="font-size: 16px; margin-bottom: 8px;">用户详情：{{ detail.user.username }}</h3>
      <dl style="display: grid; grid-template-columns: 110px 1fr; gap: 6px 12px; margin: 0;">
        <div><dt style="color: #64748b;">UID</dt><dd style="margin: 0;">{{ detail.user.uid }}</dd></div>
        <div><dt style="color: #64748b;">邮箱</dt><dd style="margin: 0;">{{ detail.user.email }}</dd></div>
        <div><dt style="color: #64748b;">昵称</dt><dd style="margin: 0;">{{ detail.user.nickname || '—' }}</dd></div>
        <div><dt style="color: #64748b;">头像</dt><dd style="margin: 0; word-break: break-all;">{{ detail.user.avatar || '—' }}</dd></div>
        <div><dt style="color: #64748b;">注册时间</dt><dd style="margin: 0;">{{ detail.user.created_at }}</dd></div>
        <div><dt style="color: #64748b;">评论数</dt><dd style="margin: 0;">{{ detail.comment_count }}</dd></div>
      </dl>
    </div>
  </div>
</template>
