import { describe, expect, it } from 'vitest'
// @ts-expect-error 构建配置为 .mjs，无类型声明
import config from './vite.config.mjs'

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
