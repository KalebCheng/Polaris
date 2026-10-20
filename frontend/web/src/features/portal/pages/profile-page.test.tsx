import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import type { User } from '@/shared/api/types'
import PortalProfilePage from './profile-page'

const mocks = vi.hoisted(() => ({
  me: vi.fn(),
  auth: {
    status: {
      authenticated: true,
      institution_id: 7,
      institution_name: 'Polaris 科技',
      name: '10002',
      display_name: '张三',
    },
    refresh: vi.fn(),
  },
}))

vi.mock('../portal-api', () => ({
  portalApi: { me: mocks.me, uploadAvatar: vi.fn() },
}))

vi.mock('@/shared/auth/auth-context', () => ({
  useAuth: () => mocks.auth,
}))

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <PortalProfilePage />
    </I18nextProvider>,
  )
}

describe('PortalProfilePage', () => {
  // 回归：个人资料「组织」曾直接回显 institution_id，用户看到的是 7 这类主键。
  it('组织一栏展示组织名称而不是组织 id', async () => {
    mocks.me.mockResolvedValue({
      user: {
        id: 11,
        uid: 'u-1',
        institution_id: 7,
        name: '10002',
        display_name: '张三',
        email: 'zhangsan@example.com',
        email_verified: true,
        phone_verified: false,
      } as User,
      roles: [],
      permissions: [],
    })
    renderPage()

    expect(await screen.findByText('Polaris 科技')).toBeInTheDocument()
    expect(screen.queryByText('7')).toBeNull()
  })
})
