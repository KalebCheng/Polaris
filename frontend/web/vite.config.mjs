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
    // 必须显式绑定回环地址：默认值 `localhost` 会原样交给 Node 的 listen()，
    // 而 Node ≥17 按 DNS 的 verbatim 顺序解析（不再把 IPv4 提前），macOS 上 localhost 常先解析到 ::1，
    // 结果**只监听 IPv6 回环**，http://127.0.0.1:5173 直接连不上。
    // 后端 auth.oauth.redirect_uri 用的正是 http://127.0.0.1:5173/sso/callback，
    // 飞书/钉钉等把浏览器回跳到 127.0.0.1 时会打不开页面（localhost 仍可访问：浏览器会从 ::1 回退到 127.0.0.1）。
    // 需要给别人 / 手机访问时临时用 `pnpm dev --host`（监听所有网卡）。
    host: '127.0.0.1',
    proxy: {
      '/api': { target: proxyTarget, changeOrigin: true },
      // 整条 /oauth/* 都是后端的 OIDC 协议端点（authorize / token / userinfo / introspect / logout），
      // 前端页面一律不占这条前缀（SSO 回调中间页在 /sso/callback），因此不需要任何"前缀例外"
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
