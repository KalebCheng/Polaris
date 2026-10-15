import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ConnectionIcon, resolveIconSlug } from './connection-icon'

describe('ConnectionIcon', () => {
  it('官方素材优先：钉钉渲染本地素材而非兜底矢量', () => {
    render(<ConnectionIcon icon={{ key: 'dingtalk' }} size={40} />)
    const img = screen.getByRole('presentation', { hidden: true }) as HTMLImageElement
    expect(img.tagName).toBe('IMG')
    expect(img.getAttribute('src')).toBe('/icons/connections/dingtalk.png')
    expect(img.getAttribute('width')).toBe('40')
  })

  it('Lark 复用飞书素材（用户确认同款）', () => {
    const spec = { key: 'feishu', by_sub_type: { lark: 'lark' } }
    expect(resolveIconSlug(spec, 'lark', 'lark')).toBe('lark')
    render(<ConnectionIcon icon={spec} type="lark" subType="lark" size={32} />)
    const img = screen.getByRole('presentation', { hidden: true }) as HTMLImageElement
    expect(img.getAttribute('src')).toBe('/icons/connections/feishu.png')
  })

  it('子类型图标优先：database 按 mysql / postgres 区分', () => {
    const spec = { key: 'database', by_sub_type: { mysql: 'mysql', postgres: 'postgres' } }
    const { unmount } = render(
      <ConnectionIcon icon={spec} type="database" subType="mysql" size={32} />,
    )
    expect(
      (screen.getByRole('presentation', { hidden: true }) as HTMLImageElement).getAttribute('src'),
    ).toBe('/icons/connections/mysql.svg')
    unmount()
    render(<ConnectionIcon icon={spec} type="database" subType="postgres" size={32} />)
    expect(
      (screen.getByRole('presentation', { hidden: true }) as HTMLImageElement).getAttribute('src'),
    ).toBe('/icons/connections/postgres.png')
  })

  it('未知类型回退内联矢量（不出现空白占位）', () => {
    const { container } = render(<ConnectionIcon type="brand-new-type" size={24} />)
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it('通用 HTTP 使用地球兜底，其余未知使用插头兜底', () => {
    const { container: httpBox } = render(<ConnectionIcon type="http" size={24} />)
    expect(httpBox.querySelector('svg')?.getAttribute('class')).toContain('lucide-globe')
    const { container: otherBox } = render(<ConnectionIcon type="whatever" size={24} />)
    expect(otherBox.querySelector('svg')?.getAttribute('class')).toContain('lucide-plug')
  })
})
