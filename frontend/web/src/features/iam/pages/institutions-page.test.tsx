import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import zh from '@/shared/i18n/locales/zh.json'
import InstitutionsPage from './institutions-page'

vi.mock('../api', () => ({
  institutionApi: {
    list: async () => ({
      items: [
        {
          id: 1,
          name: 'built-in',
          display_name: 'Polaris',
          description: '平台内置组织',
          logo: '',
          website_url: '',
          disable_signin: false,
          created_at: '2026-10-15T02:00:00Z',
        },
        {
          id: 2,
          name: 'acme',
          display_name: 'Acme',
          description: '',
          logo: '',
          website_url: 'https://acme.example.com',
          disable_signin: true,
          created_at: '2026-10-14T02:00:00Z',
        },
      ],
      total: 2,
      page: 1,
      size: 20,
    }),
    remove: vi.fn(),
  },
}))

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/institutions']}>
        <InstitutionsPage />
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('InstitutionsPage', () => {
  // 断言写字面量（zh 词条），避免用 i18n.t 与自身互证
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('组织列表是卡片网格（不再是表格），并展示图标位、标识与状态', async () => {
    renderPage()
    const cards = await screen.findAllByRole('article')
    expect(screen.queryByRole('table')).toBeNull()
    expect(cards).toHaveLength(2)

    const [polaris, acme] = cards
    // 卡片以「显示名 + 组织标识」为主标题，标识同时进徽标
    expect(within(polaris).getByText('Polaris')).toBeInTheDocument()
    expect(within(polaris).getByText('built-in')).toBeInTheDocument()
    // 主操作常驻：进入工作区（表格里它只是名称上的小链接，容易错过）
    expect(within(polaris).getByRole('link', { name: zh.institutions.enterWorkspace })).toHaveAttribute(
      'href',
      '/institutions/1',
    )
    // 内置组织标出「内置」，禁止登录的组织标出警告状态
    expect(within(polaris).getByText(zh.institutions.builtIn)).toBeInTheDocument()
    expect(within(acme).getByText(zh.institutions.disableSignin)).toBeInTheDocument()
    expect(within(polaris).queryByText(zh.institutions.disableSignin)).toBeNull()
  })

  it('内置组织不给删除入口，普通组织提供删除', async () => {
    const user = userEvent.setup()
    renderPage()
    const cards = await screen.findAllByRole('article')

    await user.click(within(cards[0]).getByRole('button', { name: zh.common.actions }))
    expect(await screen.findByRole('menuitem', { name: zh.common.edit })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: zh.common.delete })).toBeNull()
    await user.keyboard('{Escape}')

    await user.click(within(cards[1]).getByRole('button', { name: zh.common.actions }))
    expect(await screen.findByRole('menuitem', { name: zh.common.delete })).toBeInTheDocument()
  })
})
