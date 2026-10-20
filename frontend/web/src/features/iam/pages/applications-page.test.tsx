import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import zh from '@/shared/i18n/locales/zh.json'
import ApplicationsPage from './applications-page'

vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => '1',
  useWorkspaceInstitutionId: () => '1',
}))

vi.mock('../api', () => ({
  applicationApi: {
    list: async () => ({
      items: [
        {
          id: 7,
          institution_id: 1,
          code: 'portal',
          name: '用户门户',
          type: 'web',
          state: 'active',
          client_id: 'client-7',
          logo: '',
          description: '终端用户入口',
          access_mode: 'restricted',
          access_subject_count: 2,
          created_at: '2026-10-15T02:00:00Z',
          signin_methods: [
            { name: 'Password' },
            { name: 'EmailCode' },
            { name: 'SMSCode' },
            { name: 'MagicLink' },
          ],
        },
        {
          id: 8,
          institution_id: 1,
          code: 'legacy',
          name: '',
          type: 'web',
          state: 'inactive',
          client_id: 'client-8',
          access_mode: 'all',
          created_at: '2026-10-14T02:00:00Z',
          signin_methods: [],
        },
      ],
      total: 2,
      page: 1,
      size: 20,
    }),
    remove: vi.fn(),
    resetSecret: vi.fn(),
  },
}))

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/institutions/1/applications']}>
        <ApplicationsPage />
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('ApplicationsPage', () => {
  // 断言写字面量（zh 词条），避免用 i18n.t 与自身互证
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('应用列表是卡片网格（不再是表格），一张卡里放齐名称 / 标识 / 登录方式 / 客户端 ID', async () => {
    renderPage()
    const cards = await screen.findAllByRole('article')
    expect(screen.queryByRole('table')).toBeNull()
    expect(cards).toHaveLength(2)

    const [portal, legacy] = cards
    expect(within(portal).getByText('用户门户')).toBeInTheDocument()
    expect(within(portal).getByText('portal')).toBeInTheDocument()
    expect(within(portal).getByText('client-7')).toBeInTheDocument()
    // 登录方式平铺 3 个，其余折叠成「还有 N 项」
    expect(within(portal).getByText('Password')).toBeInTheDocument()
    expect(within(portal).getByText('还有 1 项')).toBeInTheDocument()
    // 未启用登录方式时不显示空白（旧表格里是一个 "-"）
    expect(within(legacy).getByText(zh.applications.noMethods)).toBeInTheDocument()
    // 停用的应用标出「禁用」
    expect(within(legacy).getByText(zh.common.disabled)).toBeInTheDocument()
    // 无 display_name 时回退到应用标识
    expect(within(legacy).getAllByText('legacy').length).toBeGreaterThan(0)
  })

  it('卡片标出「访问范围」：受限给出授权主体数，默认全员可访问', async () => {
    renderPage()
    const [portal, legacy] = await screen.findAllByRole('article')
    expect(within(portal).getByText('受限（2）')).toBeInTheDocument()
    expect(within(legacy).getByText('全员可访问')).toBeInTheDocument()
  })

  it('主操作编辑直达向导，端点 / 重置密钥 / 删除收进「更多」', async () => {
    const user = userEvent.setup()
    renderPage()
    const [portal] = await screen.findAllByRole('article')

    expect(within(portal).getByRole('link', { name: zh.common.edit })).toHaveAttribute(
      'href',
      '/institutions/1/applications/7',
    )

    await user.click(within(portal).getByRole('button', { name: zh.common.actions }))
    expect(await screen.findByRole('menuitem', { name: zh.applications.oauthEndpoints })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: zh.applications.resetSecret })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: zh.common.delete })).toBeInTheDocument()
  })
})
