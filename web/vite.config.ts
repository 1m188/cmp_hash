import { defineConfig } from 'vitest/config'

// base: './' —— 使用相对路径引用静态资源，
// 使构建产物可以直接部署在 GitHub Pages 的任意子路径（如 /cmp_hash/）下。
export default defineConfig({
  base: './',
  build: {
    target: 'es2022',
    sourcemap: false,
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
