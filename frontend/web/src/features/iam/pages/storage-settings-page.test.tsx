import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import type { ProviderCatalog, StorageSettings } from '@/shared/api/types'
import { providerApi, storageApi } from '../api'
import StorageSettingsPage from './storage-settings-page'
import { SettingsLayout } from './settings-layout'

vi.mock('../api', () => ({
  providerApi: { catalog: vi.fn() },
  storageApi: { getSettings: vi.fn(), updateSettings: vi.fn(), testSettings: vi.fn() },
}))

/** 目录：类型清单（图标 / 名称 / 说明）由后端声明，界面只按目录渲染 */
const CATALOG = {
  categories: [
    {
      key: 'storage',
      name: '存储',
      description: '组织级文件存储',
      icon: 'storage',
      singleton: true,
      types: [
        {
          key: 'local',
          name: '本地文件系统',
          description: '保存到服务器本地目录',
          icon: 'disk',
          category: 'storage',
          fields: [],
          capabilities: {
            signin: false,
            notify: false,
            captcha: false,
            storage: true,
            mfa: false,
            test: true,
          },
        },
        {
          key: 's3',
          name: 'AWS S3',
          description: '兼容 S3 协议的对象存储',
          icon: 's3',
          category: 'storage',
          fields: [],
          capabilities: {
            signin: false,
            notify: false,
            captcha: false,
            storage: true,
            mfa: false,
            test: true,
          },
        },
        {
          key: 'minio',
          name: 'MinIO',
          description: '自建 MinIO',
          icon: 'minio',
          category: 'storage',
          fields: [],
          capabilities: {
            signin: false,
            notify: false,
            captcha: false,
            storage: true,
            mfa: false,
            test: true,
          },
        },
      ],
    },
  ],
} as ProviderCatalog

const UNCONFIGURED: StorageSettings = {
  type: '',
  path_style: false,
  has_secret: false,
  updated_at: '2026-10-15T02:00:00Z',
}

const CONFIGURED: StorageSettings = {
  type: 's3',
  endpoint: 'https://s3.example.com',
  bucket: 'polaris',
  region: 'us-east-1',
  access_key: 'AKIAEXAMPLE',
  path_prefix: 'polaris/',
  path_style: true,
  has_secret: true,
  updated_at: '2026-10-15T02:00:00Z',
}

/**
 * 按真实结构渲染：页头（标题 / 描述），以及页头操作（"保存 / 取消"）都由**组级布局**承载，
 * 页面通过 portal 把按钮投进去 —— 单独渲染页面时页头不存在，按钮也就不会出现。
 */
function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/settings/storage']}>
        <Routes>
          <Route path="/settings" element={<SettingsLayout />}>
            <Route path="storage" element={<StorageSettingsPage />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

const t = i18n.t

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(providerApi.catalog).mockResolvedValue(CATALOG)
  vi.mocked(storageApi.updateSettings).mockResolvedValue(undefined)
  vi.mocked(storageApi.testSettings).mockResolvedValue(undefined)
})

describe('StorageSettingsPage', () => {
  it('未配置：状态药丸显示「未配置」，只提供「配置存储」，不给测试连通性', async () => {
    vi.mocked(storageApi.getSettings).mockResolvedValue(UNCONFIGURED)
    renderPage()

    expect(await screen.findByRole('button', { name: t('storage.configure') })).toBeInTheDocument()
    expect(screen.getAllByText(t('storage.statusUnset')).length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: t('storage.test') })).toBeNull()
    // 未配置时没有"清除"可言
    expect(screen.queryByRole('button', { name: t('storage.clearConfig') })).toBeNull()
  })

  it('已配置：卡片展示目录里的类型名与真实连接信息，并给出测试 / 修改 / 清除入口', async () => {
    vi.mocked(storageApi.getSettings).mockResolvedValue(CONFIGURED)
    renderPage()

    // 类型名与说明来自提供商目录（不是前端硬编码的类型表）
    expect(await screen.findByText('AWS S3')).toBeInTheDocument()
    expect(screen.getByText(/https:\/\/s3\.example\.com \/ polaris/)).toBeInTheDocument()
    expect(screen.getByText(t('storage.platformDefault'))).toBeInTheDocument()
    expect(screen.getAllByText(t('storage.statusActive')).length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: t('storage.test') })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: t('storage.editConfig') })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: t('storage.clearConfig') })).toBeInTheDocument()
  })

  it('测试连通性：点卡片内联入口即自检，不再需要先保存', async () => {
    vi.mocked(storageApi.getSettings).mockResolvedValue(CONFIGURED)
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: t('storage.test') }))
    expect(storageApi.testSettings).toHaveBeenCalledTimes(1)
  })

  it('编辑：类型决定字段集合，切到本地磁盘后 Bucket 消失、根目录出现', async () => {
    vi.mocked(storageApi.getSettings).mockResolvedValue(CONFIGURED)
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: t('storage.editConfig') }))
    expect(screen.getByLabelText(t('storage.bucket'))).toHaveValue('polaris')

    await userEvent.click(screen.getByRole('radio', { name: /本地文件系统/ }))
    expect(screen.queryByLabelText(t('storage.bucket'))).toBeNull()
    expect(screen.queryByLabelText(t('storage.secretKey'))).toBeNull()
    expect(screen.getByLabelText(t('storage.rootPath'))).toBeInTheDocument()
  })

  it('保存：提交体逐字段对应 storage_settings 列，密钥为 write-only', async () => {
    vi.mocked(storageApi.getSettings).mockResolvedValue(CONFIGURED)
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: t('storage.editConfig') }))
    await userEvent.clear(screen.getByLabelText(t('storage.bucket')))
    await userEvent.type(screen.getByLabelText(t('storage.bucket')), 'polaris-2')
    await userEvent.type(screen.getByLabelText(t('storage.secretKey')), 'new-secret')
    await userEvent.click(screen.getByRole('button', { name: t('common.save') }))

    expect(storageApi.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 's3',
        endpoint: 'https://s3.example.com',
        bucket: 'polaris-2',
        region: 'us-east-1',
        access_key: 'AKIAEXAMPLE',
        secret_key: 'new-secret',
        path_style: true,
      }),
    )
  })

  it('清除配置：二次确认后提交空类型（后端据此整行清零）', async () => {
    vi.mocked(storageApi.getSettings).mockResolvedValue(CONFIGURED)
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: t('storage.clearConfig') }))
    await userEvent.click(screen.getByRole('button', { name: t('common.confirm') }))

    expect(storageApi.updateSettings).toHaveBeenCalledWith({ type: '' })
  })

  it('读取失败：显式报错并可重试，绝不退化成「未配置」', async () => {
    vi.mocked(storageApi.getSettings).mockRejectedValueOnce(new Error('boom'))
    renderPage()

    expect(await screen.findByText(t('common.loadFailed'))).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: t('storage.configure') })).toBeNull()

    vi.mocked(storageApi.getSettings).mockResolvedValue(UNCONFIGURED)
    await userEvent.click(screen.getByRole('button', { name: t('common.retry') }))
    expect(await screen.findByRole('button', { name: t('storage.configure') })).toBeInTheDocument()
  })
})
