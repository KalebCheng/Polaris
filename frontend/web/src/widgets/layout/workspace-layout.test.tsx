import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { WorkspaceLayout } from './workspace-layout'

// 侧边栏是重组件，这里只关心"要不要把用户弹回组织列表"
vi.mock('./app-sidebar', () => ({ AppSidebar: () => <div>侧边栏</div> }))

const context = {
  current: {} as Record<string, unknown>,
}

vi.mock('@/features/iam/institution-context', () => ({
  useCurrentInstitution: () => context.current,
}))

function renderAt(path: string) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/institutions/:inst" element={<WorkspaceLayout />}>
            <Route path="users" element={<div>用户列表</div>} />
          </Route>
          <Route path="/institutions" element={<div>组织列表</div>} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('WorkspaceLayout', () => {
  // 回归（真实缺陷）：组织列表**还没加载完**时不能把深链接判成"组织不存在"，
  // 否则刷新 / 分享出来的工作区地址（/institutions/:id/...）一进来就被弹回组织列表。
  it('组织列表未就绪时不跳转（深链接可用）', () => {
    context.current = {
      institutions: [],
      institution: '',
      institutionId: '',
      loading: true,
      setInstitution: vi.fn(),
      reload: vi.fn(),
    }
    renderAt('/institutions/2/users')

    expect(screen.queryByText('组织列表')).toBeNull()
    expect(screen.queryByText('用户列表')).toBeNull() // 仍是加载态
  })

  it('列表已就绪且确实没有该组织 → 跳回组织列表', () => {
    context.current = {
      institutions: [{ id: 1, name: 'built-in', display_name: 'Polaris', logo: '', favicon: '' }],
      institution: '1',
      institutionId: '1',
      loading: false,
      setInstitution: vi.fn(),
      reload: vi.fn(),
    }
    renderAt('/institutions/999/users')

    expect(screen.getByText('组织列表')).toBeInTheDocument()
  })
})
