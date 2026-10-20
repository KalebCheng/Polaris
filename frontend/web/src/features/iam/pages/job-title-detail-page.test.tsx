import { render, screen, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import JobTitleDetailPage from './job-title-detail-page'
import { jobTitleApi } from '../api'

vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => '3',
  useWorkspaceInstitutionId: () => '3',
}))

vi.mock('../api', () => ({
  jobTitleApi: {
    detail: vi.fn(async () => ({
      jobTitle: {
        id: 41,
        institution_id: 3,
        name: 'AI',
        display_name: 'AI',
        users: [],
        is_enabled: true,
      },
      members: [],
      roles: [],
      permissions: [],
    })),
    setMembers: vi.fn(),
  },
  userApi: {
    list: vi.fn(async () => ({ items: [], total: 0 })),
  },
}))

describe('JobTitleDetailPage', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
  })

  it('按 jobTitle 字段渲染详情，不再误报“职务不存在”', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/institutions/3/job-titles/41']}>
          <Routes>
            <Route path="/institutions/:inst/job-titles/:id" element={<JobTitleDetailPage />} />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>,
    )

    await waitFor(() => expect(vi.mocked(jobTitleApi.detail)).toHaveBeenCalledWith(41, '3'))
    expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent('AI')
    expect(screen.queryByText(i18n.t('jobTitles.notFound'))).toBeNull()
    expect(screen.getByText(i18n.t('jobTitles.membersTitle'))).toBeInTheDocument()
    expect(screen.queryByText(/用户组/)).toBeNull()
  })
})
