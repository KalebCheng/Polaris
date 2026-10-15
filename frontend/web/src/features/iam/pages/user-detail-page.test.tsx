import { StrictMode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/shared/api/client'
import i18n from '@/shared/i18n'
import UserDetailPage from './user-detail-page'

vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => 'acme',
  useWorkspaceInstitutionId: () => '1',
}))

const user = {
  id: 7,
  uid: 'uid-7',
  institution_id: 1,
  name: 'alice',
  type: 'normal',
  display_name: '爱丽丝',
  email: 'alice@example.com',
  email_verified: true,
  phone_verified: false,
  score: 0,
  is_admin: false,
  is_forbidden: false,
  is_deleted: false,
  is_online: false,
  mfa_enabled: false,
  has_password: true,
  created_at: '2026-10-14T02:00:00Z',
  updated_at: '2026-10-14T02:00:00Z',
  department_path: 'Polaris / 研发',
}

const mocks = {
  getUser: vi.fn(async () => user),
  listDepartments: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 200 })),
  security: vi.fn(async () => ({
    password: { has_password: true, mfa_enabled: false, mfa_recovery_codes_left: 0 },
    lockouts: [],
  })),
  authorizations: vi.fn(async () => ({
    direct_roles: [],
    inherited_roles: [],
    groups: [],
    direct_permissions: [],
    effective_permissions: [],
  })),
  identities: vi.fn(async () => ({ items: [] })),
  appAccounts: vi.fn(async () => ({ items: [] })),
  provisionHistory: vi.fn(async () => ({ items: [], total: 0 })),
  records: vi.fn(async () => ({ items: [], total: 0 })),
}

vi.mock('../api', () => ({
  userApi: {
    get: (...args: unknown[]) => mocks.getUser(...(args as [])),
    security: (...args: unknown[]) => mocks.security(...(args as [])),
    authorizations: (...args: unknown[]) => mocks.authorizations(...(args as [])),
    identities: (...args: unknown[]) => mocks.identities(...(args as [])),
    appAccounts: (...args: unknown[]) => mocks.appAccounts(...(args as [])),
    provisionHistory: (...args: unknown[]) => mocks.provisionHistory(...(args as [])),
    records: (...args: unknown[]) => mocks.records(...(args as [])),
    updateProfile: vi.fn(),
    setDepartment: vi.fn(),
    setAdmin: vi.fn(),
    setDisabled: vi.fn(),
    remove: vi.fn(),
    resetPassword: vi.fn(),
    resetMfa: vi.fn(),
    unlock: vi.fn(),
    setRoles: vi.fn(),
    setGroups: vi.fn(),
  },
  departmentApi: {
    list: (...args: unknown[]) => mocks.listDepartments(...(args as [])),
  },
  groupApi: { list: vi.fn(async () => ({ items: [], total: 0 })) },
  roleApi: { list: vi.fn(async () => ({ items: [], total: 0 })) },
  sessionApi: {
    listByUser: vi.fn(async () => ({ items: [] })),
    revoke: vi.fn(),
    revokeAll: vi.fn(),
  },
}))

function renderPage() {
  return render(
    <StrictMode>
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/institutions/1/users/7']}>
          <Routes>
            <Route path="/institutions/:inst/users/:id" element={<UserDetailPage />} />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>
    </StrictMode>,
  )
}

describe('UserDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getUser.mockImplementation(async () => user)
  })

  // 回归：StrictMode 下引导请求必须**恰好一次**（第四阶段"登录页引导请求重复"的同类坑）
  it('StrictMode 下引导请求各发一次，且标签数据首次激活才加载', async () => {
    renderPage()

    expect(await screen.findAllByText('爱丽丝')).not.toHaveLength(0)
    expect(mocks.getUser).toHaveBeenCalledTimes(1)
    // 详情只读：归属来自 DTO 的 department_path，不再为"编辑部门"拉取部门列表
    expect(mocks.listDepartments).not.toHaveBeenCalled()

    // 概览标签不请求安全 / 授权 / 关联 / 活动的数据
    expect(mocks.security).not.toHaveBeenCalled()
    expect(mocks.authorizations).not.toHaveBeenCalled()
    expect(mocks.records).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('tab', { name: i18n.t('users.tabSecurity') }))
    expect(await screen.findByText(i18n.t('users.sectionLockout'))).toBeInTheDocument()
    expect(mocks.security).toHaveBeenCalledTimes(1)

    await userEvent.click(screen.getByRole('tab', { name: i18n.t('users.tabAuthorization') }))
    expect(mocks.authorizations).toHaveBeenCalledTimes(1)

    // 回到概览再进安全：不重复请求（visited 保留已挂载标签）
    await userEvent.click(screen.getByRole('tab', { name: i18n.t('users.tabOverview') }))
    await userEvent.click(screen.getByRole('tab', { name: i18n.t('users.tabSecurity') }))
    expect(mocks.security).toHaveBeenCalledTimes(1)
    expect(mocks.getUser).toHaveBeenCalledTimes(1)
  })

  // 回归（真实事故）：安全接口读不到时**必须显式报错**，绝不能退化成"没有被限制"。
  // 现场：控制台连的后端还是旧版本，/users/:id/security 404，标签页静默吞掉后显示
  // "当前没有被登录限制的登录来源"，管理员据此得出"这个人没问题"的错误结论。
  it('安全数据读不到时显式报错并可重试，不得显示空态', async () => {
    mocks.security.mockRejectedValueOnce(new ApiError('NOT_FOUND', 'page not found', 404))
    renderPage()
    await screen.findAllByText('爱丽丝')

    await userEvent.click(screen.getByRole('tab', { name: i18n.t('users.tabSecurity') }))

    // 只报一次（不是每张卡片各报一次）
    expect(await screen.findByText(i18n.t('common.loadFailed'))).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: i18n.t('common.retry') })).toHaveLength(1)
    // 关键断言：不得出现"没有限制"的空态结论
    expect(screen.queryByText(i18n.t('users.lockoutEmpty'))).toBeNull()
  })

  // 验收期修正：详情就是详情——概览只读，修改走「编辑」入口
  it('概览只读，且提供「编辑资料」入口', async () => {
    renderPage()
    expect(await screen.findAllByText('爱丽丝')).not.toHaveLength(0)

    // 只读：没有任何资料输入框，也没有保存按钮
    expect(screen.queryByLabelText(i18n.t('users.displayName'))).toBeNull()
    expect(screen.queryByRole('button', { name: i18n.t('common.save') })).toBeNull()
    // 事实照常展示（含空值占位）
    expect(screen.getByText('alice@example.com')).toBeInTheDocument()
    // 写入入口只有一个：编辑
    expect(screen.getByRole('button', { name: i18n.t('users.edit') })).toBeInTheDocument()
  })
})
