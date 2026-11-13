import { StrictMode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import type { UserImportReport } from '../api'
import UserImportPage from './user-import-page'

vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => 'acme',
  useWorkspaceInstitutionId: () => '1',
}))

const report: UserImportReport = {
  import_id: 42,
  status: 'partial',
  summary: { total: 2, created: 1, skipped: 1, failed: 0, created_departments: ['研发部/平台组'] },
  rows: [
    { row: 2, name: 'alice', email: 'alice@example.com', status: 'created', errors: [], warnings: [] },
    {
      row: 3,
      name: 'bob',
      email: '',
      status: 'skipped',
      errors: [],
      warnings: [{ column: 'name', message: 'user already exists' }],
    },
  ],
}

const mocks = {
  upload: vi.fn(async (_file: File, _options?: { autoCreateDepartments?: boolean }) => report),
  detail: vi.fn(async (_id: number, _institution?: string) => ({
    import: {
      id: 42,
      institution_id: 1,
      file_name: 'batch.csv',
      file_format: 'csv',
      status: 'running',
      total_rows: 2,
      created_count: 1,
      skipped_count: 0,
      failed_count: 0,
      auto_create_departments: false,
      operator: 'admin',
      created_at: '2026-11-12T00:00:00Z',
      updated_at: '2026-11-12T00:00:00Z',
    },
    rows: [],
  })),
  template: vi.fn(async (_lang: 'zh' | 'en') => new Blob(['x'])),
}

vi.mock('../api', () => ({
  userImportApi: {
    upload: (file: File, options?: { autoCreateDepartments?: boolean }) => mocks.upload(file, options),
    detail: (id: number, institution?: string) => mocks.detail(id, institution),
    template: (lang: 'zh' | 'en') => mocks.template(lang),
    reportUrl: (id: number) => `/api/v1/users/imports/${id}/report.csv`,
  },
}))

function renderPage() {
  return render(
    <StrictMode>
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/institutions/1/users/import']}>
          <Routes>
            <Route path="/institutions/:inst/users/import" element={<UserImportPage />} />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>
    </StrictMode>,
  )
}

describe('UserImportPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    void i18n.changeLanguage('en')
  })

  // 单阶段导入：一次请求返回完整报告，页面直接渲染逐行结果（成功 / 跳过 + 原因）
  it('uploads once and renders the per-row report', async () => {
    const user = userEvent.setup()
    renderPage()

    const file = new File(['username,display_name'], 'batch.csv', { type: 'text/csv' })
    await user.upload(document.querySelector('input[type="file"]') as HTMLInputElement, file)
    expect(screen.getByText('batch.csv')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /start import/i }))

    await waitFor(() => expect(screen.getByText('Import result')).toBeInTheDocument())
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(mocks.upload.mock.calls[0][0]).toBe(file)
    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.getByText('user already exists')).toBeInTheDocument()
    expect(screen.getByText('研发部/平台组')).toBeInTheDocument()
  })

  // 自动建部门是需要显式开启的选项：默认关闭时请求体不带该字段
  it('sends auto_create_departments only when enabled', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.upload(
      document.querySelector('input[type="file"]') as HTMLInputElement,
      new File(['x'], 'a.csv', { type: 'text/csv' }),
    )
    await user.click(screen.getByRole('switch'))
    await user.click(screen.getByRole('button', { name: /start import/i }))

    await waitFor(() => expect(mocks.upload).toHaveBeenCalledTimes(1))
    expect(mocks.upload.mock.calls[0][1]?.autoCreateDepartments).toBe(true)
  })
})
