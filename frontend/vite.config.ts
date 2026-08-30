import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 后台前端（ADR-0009）。base 设为 /admin/，使构建产物在 Go 以 /admin 前缀静态服务时
// 资产路径（/admin/assets/*）正确解析；路由用 hash 模式，避免后端 fallback。
// 产物落在 frontend/dist（标准位置）；go:embed 灌入由 Makefile 在构建时把 dist 镜像进
// backend/internal/admin/web，embed 不允许跨目录 `..`，故必须经这一步。
export default defineConfig({
  plugins: [vue()],
  base: '/admin/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})