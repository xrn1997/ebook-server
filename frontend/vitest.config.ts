import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// 默认 node 环境跑 API/组合式函数；需要 DOM 的视图用例在文件首行标注
// `// @vitest-environment happy-dom`。
export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
    // 令牌存储替身：见 test/setup.ts 的理由
    setupFiles: ['./test/setup.ts'],
  },
})
