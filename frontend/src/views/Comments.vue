<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAdminList } from '../useAdminList'
import { fetchComments, deleteComment, SUCCESS_CODE, type AdminComment } from '../api'
import Pager from '../components/Pager.vue'

const keyword = ref('')
const bookName = ref('')
/** 待二次确认的删除目标（同一时刻一个），防手滑删错 */
const pendingDelete = ref<number | null>(null)
const actionError = ref('')

const { rows, total, page, pageCount, hasPrev, hasNext, error, loading, reload, nextPage, prevPage } =
  useAdminList<AdminComment>((p) =>
    fetchComments({ ...p, keyword: keyword.value, bookName: bookName.value }),
  )

onMounted(reload)

/**
 * 换列表（筛选或翻页）时清掉二次确认与上一次的错误。
 *
 * 旧行可能已不在新页里，留着「确认删除」会让下一次点击直接删掉一个没有被重新
 * 确认过的目标——两步确认的全部意义就在这里。
 */
function resetAction(): void {
  pendingDelete.value = null
  actionError.value = ''
}

/** 筛选条件变化后回到第 1 页；上一次删除失败的提示也不该留在新结果上 */
function search(): void {
  resetAction()
  void reload()
}

/** 翻页同样是换了一批行，待确认的删除目标必须作废。 */
function changePage(move: () => Promise<void>): void {
  resetAction()
  void move()
}

/** 第一次点变为「确认删除」，再点才真正删 */
async function onDelete(comment: AdminComment): Promise<void> {
  if (pendingDelete.value !== comment.id) {
    pendingDelete.value = comment.id
    return
  }
  pendingDelete.value = null
  actionError.value = ''
  const resp = await deleteComment(comment.id)
  if (resp.code === SUCCESS_CODE) {
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

    <div class="card toolbar">
      <input v-model="keyword" placeholder="评论内容关键字" @keyup.enter="search" />
      <input v-model="bookName" placeholder="书名（精确）" @keyup.enter="search" />
      <button class="btn btn-primary" :disabled="loading" @click="search">搜索</button>
    </div>

    <p v-if="actionError" class="err">{{ actionError }}</p>
    <p v-if="loading" class="sub">加载中…</p>
    <p v-else-if="error" class="err">{{ error }}</p>
    <template v-else>
      <table>
        <thead>
          <tr><th>ID</th><th>作者</th><th>内容</th><th>章节 / 书名</th><th>时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="c in rows" :key="c.id">
            <td>{{ c.id }}</td>
            <td>{{ c.user?.username || c.user_id }}</td>
            <td class="content-cell">{{ c.content }}</td>
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
        @prev="changePage(prevPage)"
        @next="changePage(nextPage)"
      />
    </template>
  </div>
</template>

<style scoped>
.toolbar { display: flex; gap: 8px; align-items: center; }
.toolbar input { flex: 1; padding: 8px 12px; border: 1px solid #cbd5e1; border-radius: 8px; }
.content-cell { max-width: 320px; word-break: break-all; }
</style>
