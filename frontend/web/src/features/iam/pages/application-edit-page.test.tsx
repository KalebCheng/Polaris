import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import type { Application, ApplicationAccess } from '@/shared/api/types'
import ApplicationEditPage from './application-edit-page'
import { accountStrategyApi, appAccountApi, provisionHistoryApi } from '../api'

vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => '1',
  useWorkspaceInstitutionId: () => '1',
}))

vi.mock('@/features/auth/auth-context', () => ({
  useAuth: () => ({ can: () => true }),
}))

/** 已存在的应用（带合法回跳地址，保证主表单校验能过） */
const app = {
  id: 7,
  institution_id: 1,
  code: 'portal',
  name: '用户门户',
  category: 'default',
  type: 'oidc',
  code_is_immutable: true,
  order: 0,
  self_signup_enabled: false,
  signup_auto_signin: true,
  email_code_ttl_seconds: 300,
  sms_code_ttl_seconds: 300,
  magic_link_ttl_seconds: 600,
  max_sessions: 5,
  signin_error_limit: 5,
  signin_retry_wait_minutes: 15,
  grant_types: ['authorization_code'],
  redirect_uris: ['https://rp.example.com/cb'],
  access_mode: 'restricted',
  state: 'active',
  client_id: 'client-7',
  signin_methods: [{ name: 'Password', displayName: 'Password' }],
  created_at: '2026-10-15T02:00:00Z',
  updated_at: '2026-10-15T02:00:00Z',
} as unknown as Application

const accessDetail: ApplicationAccess = {
  access_mode: 'restricted',
  users: [{ name: 'zhangsan', display_name: '张三', missing: false }],
  groups: [],
  departments: [],
}

const setAccess = vi.fn(async () => accessDetail)

vi.mock('../api', () => ({
  applicationApi: {
    get: async () => app,
    access: async () => accessDetail,
    update: vi.fn(async () => undefined),
    setAccess: (...args: unknown[]) => setAccess(...(args as [])),
    create: vi.fn(),
    remove: vi.fn(),
    resetSecret: vi.fn(),
    subjectOptions: async ({ type }: { type: string }) =>
      type === 'user' ? { items: [{ name: 'lisi', display_name: '李四' }], total: 1 } : { items: [], total: 0 },
  },
  providerApi: { list: async () => ({ items: [] }) },
  certApi: { list: async () => ({ items: [] }) },
  jobTitleApi: { list: async () => ({ items: [] }) },
  accountStrategyApi: {
    list: vi.fn(async () => ({ items: [], total: 0 })),
    create: vi.fn(),
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

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/institutions/1/applications/7']}>
        <Routes>
          <Route path="/institutions/:inst/applications/:id" element={<ApplicationEditPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

/**
 * 回归：保存时必须提交**当前**的访问授权，而不是闭包里的初始值。
 *
 * 曾经的 bug：保存用的 `useCallback` 漏了 `access` 依赖 → 一直闭包住初始的
 * "全员可访问 + 空名单" → 用户明明选了一批用户、保存成功，回来还是"全员可访问"。
 */
describe('ApplicationEditPage 保存访问授权', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('回显已保存的受限名单，并在保存时把"新增后"的最新名单提交上去', async () => {
    const user = userEvent.setup()
    setAccess.mockClear()
    renderPage()

    // 等应用加载完（第 1 步的标识回填），再点步骤条最后一步「授权与访问」
    await waitFor(() => expect(screen.getByDisplayValue('portal')).toBeInTheDocument())
    const nav = screen.getByRole('navigation', { name: i18n.t('wizard.steps') })
    const steps = within(nav).getAllByRole('button')
    await user.click(steps[steps.length - 1])

    // 回显：开关处于「受限」（不是全员可访问），且已授权名单里有张三
    expect(screen.getByText('张三')).toBeInTheDocument()
    expect(screen.getByRole('switch')).toHaveAttribute('data-state', 'unchecked')

    // 再加一个用户（李四）
    await user.click(screen.getByRole('button', { name: '添加授权' }))
    await user.click(await screen.findByRole('checkbox', { name: '选择 李四' }))
    await user.click(screen.getByRole('button', { name: '确认' }))

    // 保存
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(setAccess).toHaveBeenCalled())
    expect(setAccess).toHaveBeenCalledWith(7, {
      access_mode: 'restricted',
      users: ['zhangsan', 'lisi'],
      groups: [],
      departments: [],
    })
  })
})

/**
 * 回归：三个账号生命周期视图必须收在应用详情里，并按当前应用主键过滤，
 * 不能再退回到“全局页面 + 手填应用标识”的形态。
 */
describe('ApplicationEditPage 账号供给', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('策略 / 应用账号 / 同步记录都按 application_id=7 查询', async () => {
    const user = userEvent.setup()
    vi.mocked(accountStrategyApi.list).mockClear()
    vi.mocked(appAccountApi.list).mockClear()
    vi.mocked(provisionHistoryApi.list).mockClear()
    renderPage()

    await waitFor(() => expect(screen.getByDisplayValue('portal')).toBeInTheDocument())
    await user.click(screen.getByRole('tab', { name: i18n.t('applications.tabProvisioning') }))

    await waitFor(() =>
      expect(vi.mocked(accountStrategyApi.list)).toHaveBeenCalledWith(
        expect.objectContaining({ application_id: 7 }),
      ),
    )

    await user.click(screen.getByRole('tab', { name: i18n.t('appAccounts.title') }))
    await waitFor(() =>
      expect(vi.mocked(appAccountApi.list)).toHaveBeenCalledWith(
        expect.objectContaining({ application_id: 7 }),
      ),
    )

    await user.click(screen.getByRole('tab', { name: i18n.t('provisioning.title') }))
    await waitFor(() =>
      expect(vi.mocked(provisionHistoryApi.list)).toHaveBeenCalledWith(
        expect.objectContaining({ application_id: 7 }),
      ),
    )
  })
})
