import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 认证舞台（登录 / OAuth 回调）的样式契约回归。
 *
 * 极简改版后舞台不再有星场 / 玻璃舱，但两条真实踩坑仍需锁死：
 * 1. 只重映射 `--foreground` 而不重新声明 `color` 是无效的：body 上的 `color` 早已按全局令牌
 *    解析成具体颜色再被继承，浅色主题下登录页正文会变成深色而几乎不可读；
 * 2. 认证页依赖 `@/shared/ui` 组件，主按钮 / 描边 / 文字色都由本作用域重映射的语义令牌驱动，
 *    少定义任何一个都会让按钮默默回退到控制台的紫罗兰主色，破坏"品牌深紫主按钮"的高级感。
 */
describe('认证舞台样式契约', () => {
  const css = readFileSync(join(process.cwd(), 'src/styles/globals.css'), 'utf8')
  const scope = /\.auth-scope \{[\s\S]*?\n  \}/.exec(css)?.[0] ?? ''

  it('.auth-scope 必须重新声明 color，否则浅色主题下正文不可读', () => {
    expect(scope).toContain('color: var(--foreground)')
  })

  it('.auth-scope 必须重映射核心语义令牌（舞台底色 / 主按钮 / 描边）', () => {
    for (const token of ['--background', '--foreground', '--primary', '--primary-foreground', '--border']) {
      expect(scope).toContain(`${token}:`)
    }
  })

  it('主按钮与标签选中态统一走品牌紫：--primary 直接引用 --ink', () => {
    expect(scope).toContain('--primary: var(--ink)')
    expect(scope).toContain('--primary-foreground: var(--ink-foreground)')

    const lightScope = /html:not\(\.dark\) \.auth-scope \{[\s\S]*?\n  \}/.exec(css)?.[0] ?? ''
    expect(lightScope).toContain('--primary: var(--ink)')
  })

  it('存在极简入场动画，且减弱动效时被关闭', () => {
    expect(css).toContain('.auth-rise')
    expect(css).toContain('@media (prefers-reduced-motion: reduce)')
  })
})
