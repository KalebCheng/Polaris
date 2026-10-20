import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import SignupPage from './signup-page'

vi.mock('./auth-api', () => ({
  authApi: {
    appInfo: async () => ({
      institution_id: 2,
      code: 'net-bird',
      self_signup_enabled: true,
      signup_items: [
        { name: 'Username', visible: true, required: true },
        { name: 'Password', visible: true, required: true },
      ],
      terms_of_use: '',
    }),
    appInfoByClientId: async () => ({}),
    signup: async () => ({ status: 'ok' }),
  },
}))

vi.mock('./auth-context', () => ({
  useAuth: () => ({ authenticated: false, refresh: async () => {}, isAdmin: false }),
}))

/**
 * 回归：注册表单此前带 `required` 但没关原生校验，浏览器会先弹 "Please fill out this field."
 * 原生气泡，`submit` 里的内联提示根本没机会出现。现在统一走红框 + 红色光晕。
 */
describe('注册表单必填校验', () => {
  it('必填为空时标红字段并显示内联提示，而不是浏览器原生气泡', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter>
          <SignupPage />
        </MemoryRouter>
      </I18nextProvider>,
    )
    await screen.findByLabelText(/Username/)

    await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.signUp') }))

    const username = document.getElementById('signup-username')
    expect(username).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByRole('alert')).toHaveTextContent(i18n.t('signup.fieldRequired', { field: 'Username' }))
  })
})
