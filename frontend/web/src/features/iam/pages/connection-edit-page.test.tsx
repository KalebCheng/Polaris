import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import ConnectionEditPage from './connection-edit-page'

// 组织上下文由路由/上下文提供，这里直接给定当前组织
vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => 'acme',
  useWorkspaceInstitutionId: () => '1',
}))

// 目录是类型与字段 schema 的唯一来源；测试只关心「接入方」标识的呈现位置
vi.mock('../api', () => ({
  connectionApi: {
    catalog: async () => ({
      directions: [
        {
          direction: 'in',
          types: [
            {
              key: 'dingtalk',
              direction: 'in',
              name: '钉钉',
              description: 'desc',
              icon: { key: 'dingtalk' },
              fields: [],
              capabilities: {
                mapping: false,
                incremental: false,
                group_scope: true,
                soft_delete: true,
                test: true,
                target_department: true,
              },
            },
          ],
        },
      ],
      target_fields: [],
    }),
  },
  connectorApi: { create: vi.fn(), update: vi.fn(), get: vi.fn() },
  syncerApi: {
    create: vi.fn(),
    update: vi.fn(),
    get: async () => ({
      id: 7,
      institution_id: 1,
      name: 'corp-dir',
      display_name: '企业目录',
      type: 'dingtalk',
      sub_type: '',
      is_enabled: true,
      schedule: '',
    }),
  },
}))

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/institutions/acme/connections/new?direction=in&type=dingtalk']}>
        <Routes>
          <Route path="/institutions/:inst/connections/new" element={<ConnectionEditPage />} />
          <Route path="/institutions/:inst/connections/:id" element={<ConnectionEditPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('ConnectionEditPage', () => {
  it('「接入方」只在第一步出现一次，切换步骤不重复出现', async () => {
    renderPage()
    // 目录加载完成后，第一步展示接入方（类型图标 + 名称）与「更换接入方式」
    expect(await screen.findByText('钉钉')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: i18n.t('connections.changeType') })).toBeInTheDocument()

    const nav = screen.getByRole('navigation', { name: i18n.t('wizard.steps') })
    const buttons = within(nav).getAllByRole('button')

    await userEvent.click(buttons[1])
    expect(screen.getByText(i18n.t('connections.stepConfigDesc'))).toBeInTheDocument()
    expect(screen.queryByText('钉钉')).toBeNull()
    expect(screen.queryByRole('button', { name: i18n.t('connections.changeType') })).toBeNull()

    await userEvent.click(buttons[0])
    expect(screen.getByText('钉钉')).toBeInTheDocument()
  })
})

describe('ConnectionEditPage 编辑路径', () => {
  it('直接进入向导而不是"重新选接入方式"（URL 只有 id，类型来自记录）', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/institutions/acme/connections/7?direction=in']}>
          <Routes>
            <Route path="/institutions/:inst/connections/:id" element={<ConnectionEditPage />} />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>,
    )

    // 回归：曾因"先等 typeInfo 再取记录"而死锁，编辑会停在类型选择页
    expect(await screen.findByText('钉钉')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: i18n.t('connections.addThisType') })).toBeNull()
  })
})
