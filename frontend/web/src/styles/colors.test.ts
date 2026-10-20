import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 配色令牌回归：配色不是一组孤立的 CSS 变量，必须锁定“品牌紫 + 雾紫中性色 + 语义状态色”的映射，
 * 避免后续页面又回退到橙主色 / 蓝主色，或把状态色直接当实心按钮色使用。
 */
describe('Radix 配色令牌契约', () => {
  const css = readFileSync(join(process.cwd(), 'src/styles/globals.css'), 'utf8')

  it('引入官方 Radix 色阶，并覆盖亮 / 暗主题', () => {
    for (const scale of ['violet', 'mauve', 'amber', 'jade', 'cyan', 'red']) {
      expect(css).toContain(`@import '@radix-ui/colors/${scale}.css'`)
      expect(css).toContain(`@import '@radix-ui/colors/${scale}-dark.css'`)
    }
  })

  it('品牌色使用 Violet，实心底使用 9 / 10 步，选中态使用 9 / 11 步', () => {
    expect(css).toContain('--primary: var(--violet-10)')
    expect(css).toContain('--ink: var(--violet-11)')
    expect(css).toContain('--ink: var(--violet-9)')
    expect(css).toContain('--ring: var(--violet-9)')
  })

  it('中性色使用 Mauve，画布与玻璃跟随明暗主题分层', () => {
    expect(css).toContain('--canvas: var(--mauve-2)')
    expect(css).toContain('--canvas: var(--mauve-1)')
    expect(css).toContain('--border: var(--mauve-6)')
    expect(css).toContain('--input: var(--mauve-7)')
  })

  it('状态色取 Radix 11 步，图表色板保留五色区分', () => {
    expect(css).toContain('--success-500: var(--jade-11)')
    expect(css).toContain('--warning-500: var(--amber-11)')
    expect(css).toContain('--info-500: var(--cyan-11)')
    expect(css).toContain('--destructive: var(--red-11)')
    for (const chart of ['violet', 'amber', 'cyan', 'jade', 'red']) {
      expect(css).toContain(`var(--${chart}-9)`)
    }
  })
})
