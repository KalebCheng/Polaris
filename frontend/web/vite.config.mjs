import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

const proxyTarget = process.env.VITE_PROXY_TARGET ?? 'http://localhost:8100'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: proxyTarget, changeOrigin: true },
      '/oauth': { target: proxyTarget, changeOrigin: true },
      '/.well-known': { target: proxyTarget, changeOrigin: true },
      // 本地存储直出（头像 / Logo / 站点图标的相对地址都是 /static/storage/...）：
      // 不代理的话开发环境会命中 SPA 回退（返回 index.html），图片一律裂图
      '/static': { target: proxyTarget, changeOrigin: true },
    },
  },
  build: {
    // 红线：任一分包超过 500KB（minify 后）必须处理；路由级懒加载保证首屏体积可控
    chunkSizeWarningLimit: 500,
    rollupOptions: {
      output: {
        // Vite 8（rolldown）要求 manualChunks 为函数形式
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('i18next')) return 'i18n'
          if (/node_modules\/(\.pnpm\/)?(react|react-dom|react-router|react-router-dom|scheduler)@/.test(id)) {
            return 'vendor'
          }
          return undefined
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.mjs'],
  },
})
