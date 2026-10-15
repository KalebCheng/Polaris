import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { BrandLockup } from './brand-lockup'

/**
 * 品牌锁定组合：平台字标 vs 组织品牌。
 *
 * 锁定的是**品牌来源规则**（AGENTS.md §7.4 / design-language §4.4）：
 * 控制台永远拿不到组织 Logo（`brandLogo` 为空），因此渲染平台字标；
 * 门户 / 登录页拿到组织 Logo 时，必须换成「组织 Logo + 名称」，不得混用平台字标。
 */
describe('BrandLockup', () => {
  it('无组织品牌时渲染平台字标（光球替换字母 o，且读屏能读到完整名称）', () => {
    const { container } = render(<BrandLockup />)

    expect(screen.getByText('Polaris')).toBeInTheDocument()
    // 字标被拆成 P / 光球 / laris 三段，视觉部分对读屏隐藏
    expect(container.querySelectorAll('svg')).toHaveLength(1)
  })

  it('有组织品牌时渲染组织 Logo + 名称，不出现平台字标', () => {
    const { container } = render(<BrandLockup src="https://cdn.example.com/acme.png" name="Acme 集团" />)

    expect(container.querySelector('img')).toHaveAttribute('src', 'https://cdn.example.com/acme.png')
    expect(screen.getByText('Acme 集团')).toBeInTheDocument()
    expect(screen.queryByText('Polaris')).toBeNull()
  })

  it('非平台名的自定义名称退化为「标识 + 名称」（不做字母替换）', () => {
    const { container } = render(<BrandLockup name="Acme" />)

    expect(screen.getByRole('img', { name: 'Polaris' })).toBeInTheDocument()
    expect(screen.getByText('Acme')).toBeInTheDocument()
    expect(container.querySelector('img')).toBeNull()
  })
})
