import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SubmitButton } from './submit-button'

describe('SubmitButton', () => {
  it('空闲态：整宽、可点击、可访问名称来自文案', () => {
    render(<SubmitButton>登录</SubmitButton>)
    const button = screen.getByRole('button', { name: '登录' })

    expect(button).toHaveAttribute('data-pending', 'false')
    expect(button).not.toHaveAttribute('aria-busy')
    expect(button).toBeEnabled()
    expect(button.className).toContain('w-full')
  })

  it('提交中：收成圆形并转圈，且不可重复点击', () => {
    render(<SubmitButton pending>登录</SubmitButton>)
    const button = screen.getByRole('button', { name: '登录' })

    expect(button).toHaveAttribute('data-pending', 'true')
    expect(button).toHaveAttribute('aria-busy', 'true')
    expect(button).toBeDisabled()
    // w-full 被 w-10 覆盖（twMerge），40px×40px + rounded-full 即一个圆
    expect(button.className).toContain('w-10')
    expect(button.className).not.toContain('w-full')
    expect(button.querySelector('.animate-spin')).not.toBeNull()
    // 文案保留在 DOM 里（只做透明度淡出），可访问名称在提交前后保持稳定
    expect(screen.getByText('登录')).toBeInTheDocument()
  })
})
