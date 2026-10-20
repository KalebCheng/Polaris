import type { ReactNode } from 'react'
import { render, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import OAuthCallbackPage from './oauth-callback-page'

const mocks = vi.hoisted(() => ({
  oauthCallback: vi.fn(),
  refresh: vi.fn(),
  navigateDocument: vi.fn(),
}))

vi.mock('@/shared/api/auth', () => ({
  authApi: { oauthCallback: mocks.oauthCallback },
}))

vi.mock('@/shared/lib/navigation', () => ({
  navigateDocument: mocks.navigateDocument,
}))

vi.mock('./auth-context', () => ({
  useAuth: () => ({ refresh: mocks.refresh }),
}))

// 回调页本身只验证交换请求；舞台的视觉与主题与这条回归无关。
vi.mock('./auth-shell', () => ({
  AuthShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

describe('OAuthCallbackPage', () => {
  beforeEach(() => {
    mocks.oauthCallback.mockReset()
    mocks.refresh.mockReset()
    mocks.navigateDocument.mockReset()
    mocks.oauthCallback.mockResolvedValue({
      user: { institution_id: 2, name: 'tester' },
      session: { from: '/' },
    })
  })

  it('默认 /sso/callback 不再把路径末段 callback 当作 provider', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/sso/callback?code=abc&state=xyz']}>
          <Routes>
            <Route path="/sso/callback" element={<OAuthCallbackPage />} />
            <Route path="/" element={<div>home</div>} />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>,
    )

    await waitFor(() => expect(mocks.oauthCallback).toHaveBeenCalledTimes(1))
    expect(mocks.oauthCallback).toHaveBeenCalledWith('abc', 'xyz')
    expect(mocks.oauthCallback.mock.calls[0]).toHaveLength(2)
  })

  it('外部 SSO 回调后用整页跳转续走上游 OIDC 授权', async () => {
    const target =
      '/oauth/authorize?client_id=netbird-dashboard&redirect_uri=https%3A%2F%2Fnetbird.example.com%2Foauth2%2Fcallback&state=upstream-state'
    mocks.oauthCallback.mockResolvedValue({
      user: { institution_id: 2, name: 'tester' },
      session: { from: target },
    })

    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/sso/callback?code=abc&state=xyz']}>
          <Routes>
            <Route path="/sso/callback" element={<OAuthCallbackPage />} />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>,
    )

    await waitFor(() => expect(mocks.navigateDocument).toHaveBeenCalledWith(target))
  })
})
