import { render, screen, within } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import zh from '@/shared/i18n/locales/zh.json'
import ProvidersPage from './providers-page'

vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => '1',
  useWorkspaceInstitutionId: () => '1',
}))

vi.mock('../api', () => ({
  providerApi: {
    catalog: async () => ({
      categories: [
        { key: 'oauth', name: '外部身份提供方', description: 'd', icon: 'signin', types: [] },
        { key: 'email', name: '邮件', description: 'd', icon: 'email', types: [] },
        { key: 'sms', name: '短信', description: 'd', icon: 'sms', types: [] },
        { key: 'captcha', name: '人机校验', description: 'd', icon: 'captcha', types: [] },
        { key: 'storage', name: '存储', description: 'd', icon: 'storage', singleton: true, types: [] },
      ],
    }),
    list: async () => ({
      items: [
        // 单例类别里启用中的那一个：卡片必须标出「当前生效」，并且能一键停用
        {
          id: 9,
          institution_id: 1,
          name: 'local-test',
          display_name: '本地存储',
          category: 'storage',
          type: 'local',
          state: 'active',
          config: { endpoint: '/data/test' },
          policy: {},
          has_secret: false,
          created_at: '',
          updated_at: '',
        },
      ],
      total: 1,
      page: 1,
      size: 20,
    }),
    test: vi.fn(),
    setState: vi.fn(),
    remove: vi.fn(),
  },
}))

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/institutions/1/providers?category=storage']}>
        <ProvidersPage />
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('ProvidersPage', () => {
  it('类别菜单：五个类别各带图标，且「存储」不再叫「对象存储」', async () => {
    renderPage()
    const tabs = await screen.findAllByRole('tab')
    expect(tabs).toHaveLength(5)
    for (const tab of tabs) {
      // 图标由后端目录的 CategoryInfo.Icon 声明；缺失时这里会没有 svg/img
      expect(within(tab).queryByRole('img') ?? tab.querySelector('svg')).toBeTruthy()
    }
    // 类别名走 i18n：中文为「存储」（本地目录与对象存储都归它）
    expect(zh.providers.category.storage).toBe('存储')
    expect(i18n.t('providers.category.storage')).toBe('Storage')
    expect(await screen.findByRole('tab', { name: /Storage/ })).toBeInTheDocument()
  })

  it('单例类别：启用中的卡片标出「当前生效」并提供停用入口', async () => {
    renderPage()
    // 「当前生效」徽标 + 停用快捷开关（单例类别同一组织只允许一个启用项）
    expect(await screen.findByText(i18n.t('providers.inUse'))).toBeInTheDocument()
    expect(screen.getByRole('button', { name: i18n.t('common.disable') })).toBeInTheDocument()
  })
})
