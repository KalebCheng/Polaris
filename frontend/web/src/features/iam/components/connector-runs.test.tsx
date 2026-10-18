import { render, screen, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { ConnectorRuns } from './connector-runs'
import { provisionHistoryApi } from '../api'

vi.mock('../api', () => ({
  provisionHistoryApi: {
    list: vi.fn(async () => ({ items: [], total: 0 })),
  },
}))

describe('ConnectorRuns', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('按连接器名称拉取账号同步记录', async () => {
    vi.mocked(provisionHistoryApi.list).mockClear()
    render(
      <I18nextProvider i18n={i18n}>
        <ConnectorRuns
          open
          onOpenChange={() => undefined}
          institution="1"
          connector="dingtalk-out"
          name="钉钉出方向"
        />
      </I18nextProvider>,
    )

    await waitFor(() =>
      expect(vi.mocked(provisionHistoryApi.list)).toHaveBeenCalledWith(
        expect.objectContaining({ institution: '1', connector: 'dingtalk-out' }),
      ),
    )
    expect(screen.getByText(i18n.t('connections.provisionRecordsEmpty'))).toBeInTheDocument()
  })
})
