import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { ApplicationProvisioning } from './application-provisioning'
import { accountStrategyApi } from '../../api'

vi.mock('@/shared/auth/auth-context', () => ({
  useAuth: () => ({ can: () => true }),
}))

vi.mock('../../api', () => ({
  accountStrategyApi: {
    list: vi.fn(async () => ({ items: [], total: 0 })),
    create: vi.fn(async () => ({})),
    update: vi.fn(),
    remove: vi.fn(),
  },
  appAccountApi: {
    list: vi.fn(async () => ({ items: [], total: 0 })),
    create: vi.fn(),
    retry: vi.fn(),
    remove: vi.fn(),
  },
  provisionHistoryApi: {
    list: vi.fn(async () => ({ items: [], total: 0 })),
  },
}))

describe('ApplicationProvisioning', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('新建策略时只提交当前应用主键', async () => {
    const user = userEvent.setup()
    vi.mocked(accountStrategyApi.list).mockClear()
    vi.mocked(accountStrategyApi.create).mockClear()
    render(
      <I18nextProvider i18n={i18n}>
        <ApplicationProvisioning institution="1" applicationId={7} applicationCode="portal" />
      </I18nextProvider>,
    )

    await waitFor(() =>
      expect(vi.mocked(accountStrategyApi.list)).toHaveBeenCalledWith(
        expect.objectContaining({ application_id: 7 }),
      ),
    )

    await user.click(screen.getByRole('button', { name: i18n.t('accountStrategies.add') }))
    await user.type(screen.getByLabelText(i18n.t('accountStrategies.name')), 'portal-on-create')
    await user.click(screen.getByRole('button', { name: i18n.t('common.save') }))

    await waitFor(() =>
      expect(vi.mocked(accountStrategyApi.create)).toHaveBeenCalledWith(
        expect.objectContaining({
          application_id: 7,
          name: 'portal-on-create',
        }),
      ),
    )
  })
})
