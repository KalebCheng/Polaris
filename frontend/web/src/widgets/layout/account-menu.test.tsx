import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { AccountMenu } from './account-menu'

const mocks = vi.hoisted(() => ({
  auth: {
    status: {
      authenticated: true,
      institution_id: 7,
      institution_name: 'Polaris 科技',
      name: '10002',
      display_name: '张三',
      email: 'zhangsan@example.com',
      avatar: 'https://cdn.example.com/avatar.png',
      is_admin: false,
    },
    signOut: vi.fn(),
  },
}))

vi.mock('@/shared/auth/auth-context', () => ({
  useAuth: () => mocks.auth,
}))

vi.mock('next-themes', () => ({
  useTheme: () => ({ theme: 'light', setTheme: vi.fn() }),
}))

// Radix Avatar.Image 在 jsdom 里会等图片 load 后才挂 <img>，这里只验证组件把头像地址传了下去。
vi.mock('@/shared/ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/shared/ui')>()
  return {
    ...actual,
    AvatarImage: ({ src, alt }: { src?: string; alt?: string }) =>
      src ? <img data-testid="account-avatar" src={src} alt={alt} /> : null,
  }
})

function renderMenu() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter>
        <AccountMenu />
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('AccountMenu', () => {
  // 回归：右上角只画姓名首字母、菜单第二行把组织主键当组织名展示。
  it('展示用户头像，菜单副标题使用组织名而不是组织 id', async () => {
    const user = userEvent.setup()
    renderMenu()

    expect(screen.getByTestId('account-avatar')).toHaveAttribute('src', 'https://cdn.example.com/avatar.png')

    await user.click(screen.getByRole('button', { name: /张三/ }))
    expect(await screen.findByText('Polaris 科技 · zhangsan@example.com')).toBeInTheDocument()
    expect(screen.queryByText('7 · zhangsan@example.com')).toBeNull()
  })
})
