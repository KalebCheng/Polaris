import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { PortalLayout } from './portal-layout'

vi.mock('@/shared/auth/auth-context', () => ({
  useAuth: () => ({
    status: {
      authenticated: true,
      institution_id: 1,
      institution_name: '因朔桔（上海）智能科技有限公司',
      institution_logo: 'https://cdn.example.com/org-logo.png',
      name: 'chengkai',
      display_name: '成凯',
    },
    signOut: vi.fn(),
  }),
}))

vi.mock('@/widgets/layout/account-menu', () => ({
  AccountMenu: () => <div data-testid="account-menu" />,
}))

describe('PortalLayout', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('uses the platform wordmark and shows the organization badge instead of the portal label', () => {
    const { container } = render(
      <MemoryRouter initialEntries={['/portal']}>
        <I18nextProvider i18n={i18n}>
          <PortalLayout />
        </I18nextProvider>
      </MemoryRouter>,
    )

    expect(screen.getByText('Polaris')).toBeInTheDocument()
    expect(screen.queryByText('用户门户')).not.toBeInTheDocument()
    expect(screen.getByText('因朔桔（上海）智能科技有限公司')).toBeInTheDocument()
    expect(container.querySelector('img[src="https://cdn.example.com/org-logo.png"]')).toBeInTheDocument()
  })
})
