<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { login, SUCCESS_CODE, token } from '../api'

const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')

async function submit(): Promise<void> {
  error.value = ''
  const resp = await login(username.value, password.value)
  const issued = resp.data?.token
  if (resp.code === SUCCESS_CODE && issued) {
    token.set(issued)
    // token.set 在 localStorage 不可用/配额满时静默失败；继续跳转只会被路由守卫弹回
    // 登录页，用户看不到任何原因。以读回值确认令牌真的落库了。
    if (token.value !== issued) {
      error.value = '无法保存登录状态，请检查浏览器是否禁用了本地存储'
      return
    }
    router.push('/')
  } else {
    error.value = resp.error || '登录失败'
  }
}
</script>

<template>
  <form class="card" @submit.prevent="submit">
    <h1>ebook 后台登录</h1>
    <input v-model="username" placeholder="账号" autocomplete="username" />
    <input v-model="password" type="password" placeholder="密码" autocomplete="current-password" />
    <p v-if="error" class="err">{{ error }}</p>
    <button type="submit">登 录</button>
  </form>
</template>

<style scoped>
.card { background: #fff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 32px; width: 320px; display: flex; flex-direction: column; gap: 12px; }
h1 { font-size: 20px; margin: 0 0 8px; }
input { padding: 10px 12px; border: 1px solid #cbd5e1; border-radius: 8px; }
button { padding: 10px; background: #0f172a; color: #fff; border: none; border-radius: 8px; font-size: 15px; }
.err { color: #dc2626; font-size: 13px; margin: 0; }
</style>