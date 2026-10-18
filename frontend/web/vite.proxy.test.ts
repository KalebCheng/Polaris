import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
// @ts-expect-error 构建配置为 .mjs，无类型声明
import config from './vite.config.mjs'

/** vitest 的 import.meta.url 不是 file:// URL，按 vite 项目的 cwd（frontend/web）定位仓库根 */
const repoFile = (relative: string) => readFileSync(resolve(process.cwd(), relative), 'utf8')

/** 后端自有的根路径前缀：管理 API、OIDC 协议面、发现文档、静态直出、探针 */
const BACKEND_PREFIXES = ['/api/', '/oauth/', '/.well-known/', '/static/', '/healthz', '/readyz']

/**
 * 回归：开发代理必须覆盖后端自有的路径前缀。
 * 曾遗漏 `/static`：上传后的相对地址（/static/storage/...）在 5173 命中 SPA 回退返回 index.html，
 * 界面上头像/Logo/站点图标全部裂图，而直连后端却是 200。
 */
describe('vite 开发代理', () => {
  const proxy = (config as { server?: { proxy?: Record<string, unknown> } }).server?.proxy ?? {}

  it('覆盖后端自有的路径前缀', () => {
    for (const prefix of ['/api', '/oauth', '/.well-known', '/static']) {
      expect(Object.keys(proxy), `缺少 ${prefix} 的代理配置`).toContain(prefix)
    }
  })
})

/**
 * 回归：dev server 必须显式绑定回环地址，不能依赖默认的 `localhost`。
 * 默认值会让 Node 只监听 ::1（IPv6），而平台配置里的回调地址是 http://127.0.0.1:5173/sso/callback——
 * IdP 把浏览器回跳到 127.0.0.1 时页面直接打不开（表现为"授权成功了但回不到应用"）。
 */
describe('vite dev server 监听地址', () => {
  const host = (config as { server?: { host?: string | boolean } }).server?.host

  it('显式设置 server.host 且不是有歧义的 localhost', () => {
    expect(host, '必须显式设置 server.host').toBeDefined()
    expect(host).not.toBe('localhost')
  })
})

/**
 * 回归：生产 nginx 的反代前缀必须与 dev 代理同一套（否则"开发好用、上线 404/裂图"）。
 * nginx 用一条正则 location 覆盖这些前缀，任何一处漏掉都是线上才暴露的故障。
 */
describe('生产 nginx 反代前缀', () => {
  const nginx = repoFile('../../docker/nginx.conf')
  const locationLine = nginx.split('\n').find((line) => line.includes('location ~ ^/(api/')) ?? ''

  it('覆盖全部后端自有前缀', () => {
    expect(locationLine, 'nginx 缺少通用反代 location').not.toBe('')
    for (const prefix of BACKEND_PREFIXES) {
      expect(locationLine, `nginx 反代缺少 ${prefix}`).toContain(prefix.replace(/^\//, ''))
    }
  })
})

/**
 * 命名空间纪律（零例外）：前端页面路由不得占用后端的自有前缀。
 *
 * 后端约定（internal/server/router.go）：管理 API 挂 /api/v1，**根路径只留给协议面与探针**。
 * 这些前缀在 dev（vite 代理）与生产（nginx 反代）都会转发给后端，前端页面若落在其中，
 * 用户看到的就是后端的 "404 page not found"——历史故障：SSO 回调中间页曾挂在 /oauth/callback。
 * 该页面现已搬到前端自己的命名空间 /sso/callback，因此这条纪律**零例外**。
 */
describe('路由命名空间：前端页面零占用后端前缀', () => {
  const declared = [...repoFile('src/Router.tsx').matchAll(/<Route\s+path="([^"]*)"/g)].map((m) => m[1])

  it('没有任何前端路由落在后端自有前缀下', () => {
    const conflicts = declared.filter((path) => BACKEND_PREFIXES.some((prefix) => path.startsWith(prefix)))
    expect(
      conflicts,
      `这些前端路由落在后端自有前缀下，会被反代转发到后端（页面直接 404）：${conflicts.join(', ')}`,
    ).toEqual([])
  })

  it('SSO 回调中间页挂在前端命名空间 /sso/callback（别再搬回 /oauth/*）', () => {
    expect(declared, '回调中间页必须挂在 /sso/callback 下').toContain('/sso/callback')
    expect(declared).toContain('/sso/callback/:provider')
  })
})
