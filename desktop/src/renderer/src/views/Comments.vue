<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAdminList } from '../useAdminList'
import { fetchComments, deleteComment, type AdminComment } from '../api'
import Pager from '../components/Pager.vue'

const keyword = ref('')
const bookName = ref('')
/** 待二次确认的删除目标（同一时刻一个），防手滑删错 */
const pendingDelete = ref<number | null>(null)
const actionError = ref('')

const { rows, total, page, pageCount, hasPrev, hasNext, error, reload, nextPage, prevPage } =
  useAdminList<AdminComment>((p) =>
    fetchComments({ ...p, keyword: keyword.value, bookName: bookName.value }),
  )

onMounted(reload)

function search() {
  void reload()
}

/** 第一次点变为「确认删除」，再点才真正删（不弹浏览器确认框，便于测试也少一步弹窗） */
async function onDelete(comment: AdminComment) {
  if (pendingDelete.value !== comment.id) {
    pendingDelete.value = comment.id
    return
  }
  pendingDelete.value = null
  const resp = await deleteComment(comment.id)
  if (resp.code === '00000') {
    await reload()
  } else {
    actionError.value = resp.error || '删除评论失败'
  }
}
</script>

<template>
  <div>
    <h2>评论管理</h2>
    <p class="sub">评论列表：按内容搜索、按书名过滤，可删除违规评论</p>

    <div class="card" style="display: flex; gap: 8px; align-items: center;">
      <input v-model="keyword" placeholder="评论内容关键字" style="flex: 1;" @keyup.enter="search" />
      <input v-model="bookName" placeholder="书名（精确）" style="flex: 1;" @keyup.enter="search" />
      <button class="btn btn-primary" @click="search">搜索</button>
    </div>

    <p v-if="actionError" class="err">{{ actionError }}</p>
    <p v-if="error" class="err">{{ error }}</p>
    <template v-else>
      <table>
        <thead>
          <tr><th>ID</th><th>作者</th><th>内容</th><th>章节 / 书名</th><th>时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="c in rows" :key="c.id">
            <td>{{ c.id }}</td>
            <td>{{ c.user?.username || c.user_id }}</td>
            <td style="max-width: 320px; word-break: break-all;">{{ c.content }}</td>
            <td>{{ c.chapter_name || c.book_name || '书籍级' }}</td>
            <td>{{ c.created_at }}</td>
            <td>
              <button
                class="btn"
                :class="pendingDelete === c.id ? 'btn-danger' : ''"
                @click="onDelete(c)"
              >
                {{ pendingDelete === c.id ? '确认删除' : '删除' }}
              </button>
            </td>
          </tr>
          <tr v-if="rows.length === 0">
            <td colspan="6" class="sub">没有匹配的评论</td>
          </tr>
        </tbody>
      </table>

      <Pager
        :page="page"
        :page-count="pageCount"
        :total="total"
        unit="条评论"
        :has-prev="hasPrev"
        :has-next="hasNext"
        @prev="prevPage"
        @next="nextPage"
      />
    </template>
  </div>
</template>
