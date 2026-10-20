import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import type { User } from '@/shared/api/types'
import PortalSecurityPage from './security-page'

const mocks = vi.hoisted(() => ({
  me: vi.fn(),
  mfaOptions: vi.fn(),
  changePassword: vi.fn(),
  navigate: vi.fn(),
  signOut: vi.fn(),
}))

vi.mock('../portal-api', () => ({
  portalApi: { me: mocks.me },
}))

vi.mock('@/shared/api/auth', () => ({
  authApi: {
    mfaOptions: mocks.mfaOptions,
    changePassword: mocks.changePassword,
  },
}))

vi.mock('@/shared/auth/auth-context', () => ({
  useAuth: () => ({ signOut: mocks.signOut }),
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => mocks.navigate }
})

function userWithPassword(hasPassword: boolean): User {
  return {
    id: 11,
    uid: 'u-1',
    institution_id: 7,
    name: 'sso-user',
    display_name: '张三',
    email: 'zhangsan@example.com',
    email_verified: true,
    phone_verified: false,
    has_password: hasPassword,
    mfa_enabled: false,
  } as User
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <PortalSecurityPage />
    </I18nextProvider>,
  )
}

describe('PortalSecurityPage', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage('zh')
    mocks.mfaOptions.mockResolvedValue({
      required: false,
      can_skip: true,
      providers: null,
      bound: null,
      remember_hours: 0,
    })
    mocks.changePassword.mockResolvedValue(undefined)
    mocks.signOut.mockResolvedValue(undefined)
  })

  // 回归：SSO 自动建号没有密码，首次设密不应要求输入当前密码，也不应提交 old_password。
  it('未设置密码时隐藏当前密码并直接提交新密码', async () => {
    mocks.me.mockResolvedValue({ user: userWithPassword(false), roles: [], permissions: [] })
    const user = userEvent.setup()
    renderPage()

    const newPassword = await screen.findByLabelText('新密码')
    expect(screen.queryByLabelText('当前密码')).toBeNull()
    expect(screen.getByText(/当前账号尚未设置密码/)).toBeInTheDocument()

    await user.type(newPassword, 'InitPass123')
    await user.type(screen.getByLabelText('确认新密码'), 'InitPass123')
    await user.click(screen.getByRole('button', { name: '设置密码' }))

    await waitFor(() => {
      expect(mocks.changePassword).toHaveBeenCalledWith({ new_password: 'InitPass123' })
    })
  })

  it('已有密码时仍展示当前密码输入框', async () => {
    mocks.me.mockResolvedValue({ user: userWithPassword(true), roles: [], permissions: [] })
    renderPage()

    expect(await screen.findByLabelText('当前密码')).toBeInTheDocument()
  })

  // MFA 选项接口失败不应连带误判密码状态。
  it('MFA 选项加载失败时仍按账号实际状态渲染密码表单', async () => {
    mocks.me.mockResolvedValue({ user: userWithPassword(false), roles: [], permissions: [] })
    mocks.mfaOptions.mockRejectedValue(new Error('mfa unavailable'))
    renderPage()

    expect(await screen.findByLabelText('新密码')).toBeInTheDocument()
    expect(screen.queryByLabelText('当前密码')).toBeNull()
  })
})
