import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const adminOrigin = process.env.ADMIN_API_ORIGIN ?? 'http://127.0.0.1:9091'

// 后台前端（ADR-0009）。base 设为 /admin/，使构建产物在 Go 以 /admin 前缀静态服务时
// 资产路径（/admin/assets/*）正确解析；路由用 hash 模式，避免后端 fallback。
// 产物落在 frontend/dist（标准位置）；go:embed 灌入由 Makefile 在构建时把 dist 镜像进
// backend/internal/admin/web，embed 不允许跨目录 `..`，故必须经这一步。
export default defineConfig({
  plugins: [vue()],
  base: '/admin/',
  server: {
    // 开发期把后台 API 与 Swagger 转发到后台引擎：生产里页面与 API 同源，
    // dev server 没有同源后端，不代理的话登录/列表请求与「API 文档」页都会 404。
    // 目标默认取后台默认监听地址；改了 admin.listen_port 用 ADMIN_API_ORIGIN 指过来。
    proxy: {
      '/admin/api': { target: adminOrigin },
      '/api-docs': { target: adminOrigin },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})