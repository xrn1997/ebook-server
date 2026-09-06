import { createRouter, createWebHashHistory } from 'vue-router'
import Overview from './views/Overview.vue'
import Config from './views/Config.vue'
import Service from './views/Service.vue'
import Users from './views/Users.vue'
import Comments from './views/Comments.vue'
import Logs from './views/Logs.vue'

const routes = [
  { path: '/', name: 'overview', component: Overview },
  { path: '/config', name: 'config', component: Config },
  { path: '/service', name: 'service', component: Service },
  { path: '/users', name: 'users', component: Users },
  { path: '/comments', name: 'comments', component: Comments },
  { path: '/logs', name: 'logs', component: Logs },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

export default router
