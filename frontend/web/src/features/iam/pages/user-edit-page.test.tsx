import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import UserEditPage from './user-edit-page'

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
  phone: '',
  phone_verified: false,
  score: 0,
  is_admin: false,
  is_forbidden: false,
  is_deleted: false,
  is_online: false,
  mfa_enabled: false,
  has_password: true,
  department_id: 3,
  created_at: '2026-10-14T02:00:00Z',
  updated_at: '2026-10-14T02:00:00Z',
}

const mocks = {
  getUser: vi.fn(async () => user),
  updateProfile: vi.fn(async () => user),
  setDepartment: vi.fn(async () => undefined),
  setAdmin: vi.fn(async () => undefined),
  create: vi.fn(async (_body: { name: string; [key: string]: unknown }) => ({ ...user, id: 99 })),
}

vi.mock('../api', () => ({
  userApi: {
    get: (...args: unknown[]) => mocks.getUser(...(args as [])),
    updateProfile: (...args: unknown[]) => mocks.updateProfile(...(args as [])),
    setDepartment: (...args: unknown[]) => mocks.setDepartment(...(args as [])),
    setAdmin: (...args: unknown[]) => mocks.setAdmin(...(args as [])),
    create: (...args: unknown[]) => mocks.create(...(args as [{ name: string; [key: string]: unknown }])),
  },
  departmentApi: {
    list: vi.fn(async () => ({
      items: [{ id: 3, name: 'rd', name_path: 'Polaris / 研发' }],
      total: 1,
      page: 1,
      size: 200,
    })),
  },
}))

function renderPage(path: string) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/institutions/:inst/users/new" element={<UserEditPage />} />
          <Route path="/institutions/:inst/users/:id/edit" element={<UserEditPage />} />
          <Route path="/institutions/:inst/users/:id" element={<div>用户详情页</div>} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('UserEditPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getUser.mockImplementation(async () => user)
  })

  // 回归 D-1：只改一个字段时，请求体里**只有**那个字段
  it('编辑：只提交发生变化的字段，改部门与管理员走各自端点', async () => {
    renderPage('/institutions/1/users/7/edit')
    const displayName = await screen.findByLabelText(i18n.t('users.displayName'))

    await userEvent.clear(displayName)
    await userEvent.type(displayName, '爱丽丝2')
    await userEvent.click(screen.getAllByRole('button', { name: i18n.t('common.save') })[0])

    expect(mocks.updateProfile).toHaveBeenCalledTimes(1)
    expect(mocks.updateProfile).toHaveBeenCalledWith(7, { display_name: '爱丽丝2' })
    expect(mocks.setDepartment).not.toHaveBeenCalled()
    expect(mocks.setAdmin).not.toHaveBeenCalled()
    expect(await screen.findByText('用户详情页')).toBeInTheDocument()
  })

  // 返工后的最终契约：单页多列分区表单——一屏看完、一行铺满；没有步骤条、没有右栏凑数卡片
  it('单页分区表单：字段成组在多列栅格内，且不出现步骤条', async () => {
    renderPage('/institutions/1/users/7/edit')
    const displayName = await screen.findByLabelText(i18n.t('users.displayName'))

    const grid = displayName.closest('.grid')
    expect(grid).not.toBeNull()
    expect(grid?.className).toMatch(/grid-cols-/)

    // 不再有向导
    expect(screen.queryByRole('navigation', { name: i18n.t('wizard.steps') })).toBeNull()

    // 四个语义分区一屏并列，标题各只出现一次
    for (const key of [
      'users.sectionBasic',
      'users.sectionContact',
      'users.sectionMembership',
      'users.sectionProfileOther',
    ]) {
      expect(screen.getAllByText(i18n.t(key))).toHaveLength(1)
    }
  })

  // 新建是"整页表单"，一次把资料填全（不再只给 5 个字段）
  it('新建：整页表单提交完整资料字段', async () => {
    renderPage('/institutions/1/users/new')
    await userEvent.type(await screen.findByLabelText(i18n.t('users.name')), 'bob')
    await userEvent.type(screen.getByLabelText(i18n.t('users.displayName')), '鲍勃')
    // 新建时多一个「初始密码」字段（编辑态没有）
    expect(screen.getByLabelText(i18n.t('users.password'))).toBeInTheDocument()

    await userEvent.click(screen.getAllByRole('button', { name: i18n.t('common.create') })[0])

    expect(mocks.create).toHaveBeenCalledTimes(1)
    const payload = mocks.create.mock.calls[0][0]
    expect(payload.name).toBe('bob')
    expect(payload.display_name).toBe('鲍勃')
    // 关键：整页表单承载的字段必须全部在请求体里（否则就是"表单能填、后端丢弃"）
    for (const field of [
      'first_name',
      'last_name',
      'email',
      'phone',
      'country_code',
      'location',
      'address',
      'affiliation',
      'title',
      'homepage',
      'bio',
      'language',
      'gender',
      'birthday',
      'education',
      'avatar',
    ]) {
      expect(payload).toHaveProperty(field)
    }
  })

  // 密码不在本页改：编辑态不出现「初始密码」，而是提示去详情页「安全与认证」重置
  it('编辑态不提供初始密码输入，只给重置入口提示', async () => {
    renderPage('/institutions/1/users/7/edit')
    await screen.findByLabelText(i18n.t('users.displayName'))

    expect(screen.queryByLabelText(i18n.t('users.password'))).toBeNull()
    expect(screen.getByText(i18n.t('users.asidePasswordNote'))).toBeInTheDocument()
  })
})
