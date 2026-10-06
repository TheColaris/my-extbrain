import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': new URL('./src', import.meta.url).pathname,
    },
  },
  server: {
    // 本地开发代理到 server 仓（默认 compose 起的 8080；VITE_BACKEND 可覆盖，多实例并行 dev 用）
    proxy: {
      '/api': process.env.VITE_BACKEND ?? 'http://127.0.0.1:8080',
      '/guide.md': process.env.VITE_BACKEND ?? 'http://127.0.0.1:8080',
      '/guide-mcp.md': process.env.VITE_BACKEND ?? 'http://127.0.0.1:8080',
    },
  },
})
