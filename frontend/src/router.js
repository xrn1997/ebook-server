import { createRouter, createWebHashHistory } from 'vue-router'
import { token } from './api.js'
import Login from './views/Login.vue'
import Dashboard from './views/Dashboard.vue'
import Users from './views/Users.vue'
import Comments from './views/Comments.vue'
import Logs from './views/Logs.vue'

const routes = [
  { path: '/login', name: 'login', component: Login },
  { path: '/', component: Dashboard },
  { path: '/users', component: Users },
  { path: '/comments', component: Comments },
  { path: '/logs', component: Logs },
]

const router = createRouter({
  // hash 模式：只加载一次 index.html，无需后端 fallback（适配 go:embed 静态服务）。
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to) => {
  if (to.name !== 'login' && !token.value) return { name: 'login' }
})

export default router