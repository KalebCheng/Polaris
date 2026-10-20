import { beforeAll, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import i18n from '@/shared/i18n'
import { NavSearch } from './nav-search'

// 权限门禁与组织上下文：本用例只关心"能不能搜到、搜到的点了去哪"
let can: (resource: string, action: string) => boolean = () => true
let institutionId = '1'

vi.mock('@/shared/auth/auth-context', () => ({
  useAuth: () => ({ can: (resource: string, action: string) => can(resource, action) }),
}))
vi.mock('@/features/iam/institution-context', () => ({
  useCurrentInstitution: () => ({ institutionId, institution: institutionId }),
}))

function renderSearch() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/']}>
        <NavSearch />
        <Routes>
          <Route path="/" element={<div>平台总览页</div>} />
          <Route path="/institutions/1/users" element={<div>用户列表页</div>} />
          <Route path="/settings/*" element={<div>设置页</div>} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

async function openAndType(keyword: string) {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: '快速跳转' }))
  const input = await screen.findByRole('textbox', { name: '快速跳转' })
  await user.type(input, keyword)
  return user
}

/**
 * 全局快速跳转：顶栏搜索面板。
 *
 * 这个面板是"两级导航（平台级 + 当前组织）拍平后的可搜索入口"，
 * 因此它必须与侧边栏/顶栏**同权**——菜单里看得见的才搜得到。
 */
describe('NavSearch', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('按关键字过滤，点击结果即跳转到该页面', async () => {
    can = () => true
    institutionId = '1'
    renderSearch()

    const user = await openAndType('用户')

    // 平台级目标（如"组织管理"）不匹配关键字，不应出现在结果里
    expect(screen.queryByRole('option', { name: '组织管理' })).toBeNull()

    const option = screen.getByRole('option', { name: '用户' })
    await user.click(option)

    expect(await screen.findByText('用户列表页')).toBeInTheDocument()
  })

  it('没有读权限的组织资源不进入结果（与导航同一套门禁）', async () => {
    // 只禁掉「职务」，其余组织内资源保持可见（空关键字 → 列出全部，便于直接断言门禁）
    can = (resource: string) => resource !== 'job_title'
    institutionId = '1'
    renderSearch()

    // 空关键字 → 列出全部目标，便于直接断言"同一套 can() 门禁"
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '快速跳转' }))
    await screen.findByRole('textbox', { name: '快速跳转' })

    expect(screen.queryByRole('option', { name: '职务' })).toBeNull()
    expect(screen.getByRole('option', { name: '部门' })).toBeInTheDocument()
  })

  it('「设置」的二级页直接可搜：审计日志 / 任务中心不再是顶级菜单，但仍能直达', async () => {
    can = () => true
    institutionId = '1'
    renderSearch()

    // 父级「系统设置」不单独列出，因此「审计日志」只有一条结果（同址两条会撞 React key）
    const user = await openAndType('审计')
    const options = screen.getAllByRole('option', { name: '审计日志' })
    expect(options).toHaveLength(1)

    await user.click(options[0])
    expect(await screen.findByText('设置页')).toBeInTheDocument()
  })

  it('没有当前组织时只列平台级目标', async () => {
    can = () => true
    institutionId = ''
    renderSearch()

    await openAndType('用户')

    expect(screen.queryByRole('option', { name: '用户' })).toBeNull()
  })
})
