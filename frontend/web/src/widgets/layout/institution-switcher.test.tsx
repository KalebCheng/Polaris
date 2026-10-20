import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { InstitutionSwitcher } from './institution-switcher'

const context = {
  institutions: [
    { id: 1, name: 'acme', display_name: 'Acme', logo: 'https://cdn.example.com/acme.png' },
    { id: 2, name: 'beta', display_name: 'Beta', logo: '' },
  ],
  institutionId: '1',
  setInstitution: vi.fn(),
  loading: false,
}

vi.mock('@/features/iam/institution-context', () => ({
  useCurrentInstitution: () => context,
}))

function renderSwitcher() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter>
        <InstitutionSwitcher />
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('InstitutionSwitcher', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('当前组织和下拉选项优先展示组织 logo，缺失时回退首字母', async () => {
    const user = userEvent.setup()
    renderSwitcher()

    const trigger = screen.getByRole('button', { name: '切换组织，当前组织：Acme' })
    expect(trigger.querySelector('img')).toHaveAttribute('src', 'https://cdn.example.com/acme.png')

    await user.click(trigger)

    const acme = await screen.findByRole('menuitem', { name: /Acme/ })
    expect(acme.querySelector('img')).toHaveAttribute('src', 'https://cdn.example.com/acme.png')

    const beta = screen.getByRole('menuitem', { name: /Beta/ })
    expect(beta.querySelector('img')).toBeNull()
    expect(within(beta).getByText('B')).toBeInTheDocument()
  })
})
