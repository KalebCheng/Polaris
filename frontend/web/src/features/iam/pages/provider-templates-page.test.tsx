import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import type { Provider, ProviderTemplate, TemplateSceneInfo } from '@/shared/api/types'
import ProviderTemplatesPage from './provider-templates-page'
import { providerApi } from '../api'

Object.defineProperty(HTMLElement.prototype, 'hasPointerCapture', {
  configurable: true,
  value: () => false,
})
Object.defineProperty(HTMLElement.prototype, 'releasePointerCapture', {
  configurable: true,
  value: () => undefined,
})

vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => 'acme',
  useWorkspaceInstitutionId: () => '1',
}))

vi.mock('../api', () => ({
  providerApi: {
    get: vi.fn(),
    templateScenes: vi.fn(),
    templates: vi.fn(),
    createTemplate: vi.fn(),
    updateTemplate: vi.fn(),
    removeTemplate: vi.fn(),
  },
}))

// Tiptap/ProseMirror 依赖真实布局，页面测试只关心表单契约；编辑器行为由后端清洗与单元测试覆盖。
vi.mock('../components/email-rich-text-editor', async () => {
  const React = await import('react')
  const Editor = React.forwardRef(function MockEmailEditor(
    {
      value,
      onChange,
      placeholder,
    }: {
      value: string
      onChange: (next: string) => void
      placeholder?: string
    },
    ref,
  ) {
    React.useImperativeHandle(
      ref,
      () => ({
        insertText: (text: string) => onChange(value + text),
        insertHTML: (html: string) => onChange(value + html),
      }),
      [onChange, value],
    )
    return React.createElement('textarea', {
      'aria-label': 'rich-text',
      value,
      placeholder,
      onChange: (event: React.ChangeEvent<HTMLTextAreaElement>) => onChange(event.target.value),
    })
  })
  return { EmailRichTextEditor: Editor }
})

const SMS_PROVIDER: Provider = {
  id: 7,
  institution_id: 1,
  name: 'sms-primary',
  display_name: '主短信通道',
  category: 'sms',
  type: 'aliyun',
  state: 'active',
  config: {},
  policy: {},
  has_secret: true,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
}

const EMAIL_PROVIDER: Provider = {
  ...SMS_PROVIDER,
  id: 8,
  name: 'mail-primary',
  display_name: '主邮件通道',
  category: 'email',
  type: 'smtp',
}

const SCENES: TemplateSceneInfo[] = [
  {
    channel: 'sms',
    scene: 'login_code',
    label: '短信登录验证码',
    variables: [
      { key: 'code', label: '验证码', required: true },
      { key: 'minutes', label: '有效分钟数' },
      { key: 'phone', label: '接收号码' },
    ],
  },
  {
    channel: 'sms',
    scene: 'mfa_code',
    label: '短信 MFA 验证码',
    variables: [
      { key: 'code', label: '验证码', required: true },
      { key: 'minutes', label: '有效分钟数' },
    ],
  },
  {
    channel: 'email',
    scene: 'login_code',
    label: '邮件登录验证码',
    variables: [
      { key: 'code', label: '验证码', required: true },
      { key: 'minutes', label: '有效分钟数' },
    ],
  },
  {
    channel: 'email',
    scene: 'magic_link',
    label: '邮件魔法链接',
    variables: [
      { key: 'link', label: '魔法链接', required: true },
      { key: 'minutes', label: '有效分钟数' },
    ],
  },
]

const SMS_TEMPLATE: ProviderTemplate = {
  id: 11,
  institution_id: 1,
  provider_id: 7,
  channel: 'sms',
  name: '登录验证码',
  scene: 'login_code',
  template_code: 'SMS_1',
  template_params: [{ key: 'code1', source: 'code' }],
  subject: '',
  content: '',
  is_default: true,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
}

function renderPage() {
  render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/institutions/acme/providers/7/templates?scene=login_code']}>
        <Routes>
          <Route path="/institutions/:inst/providers/:id/templates" element={<ProviderTemplatesPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('ProviderTemplatesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(providerApi.get).mockResolvedValue(SMS_PROVIDER)
    vi.mocked(providerApi.templateScenes).mockResolvedValue({ scenes: SCENES })
    vi.mocked(providerApi.templates).mockResolvedValue({ items: [SMS_TEMPLATE] })
    vi.mocked(providerApi.createTemplate).mockResolvedValue(SMS_TEMPLATE)
    vi.mocked(providerApi.updateTemplate).mockResolvedValue(SMS_TEMPLATE)
    vi.mocked(providerApi.removeTemplate).mockResolvedValue(undefined)
  })

  it('按场景展示独立模板列表与参数映射摘要', async () => {
    renderPage()
    expect(await screen.findByText(i18n.t('providers.templates.pageTitle'))).toBeInTheDocument()
    expect(await screen.findByText('登录验证码')).toBeInTheDocument()
    expect(screen.getByText('code1 ← code')).toBeInTheDocument()
    expect(screen.getByText(i18n.t('providers.templates.scene.mfa_code'))).toBeInTheDocument()
    expect(screen.getByText(i18n.t('providers.templates.defaultBadge'))).toBeInTheDocument()
  })

  it('短信模板按厂商变量到平台占位符的映射提交', async () => {
    vi.mocked(providerApi.templates).mockResolvedValue({ items: [] })
    const user = userEvent.setup()
    renderPage()

    const addButtons = await screen.findAllByRole('button', { name: i18n.t('providers.templates.add') })
    await user.click(addButtons[0])

    await user.type(screen.getByLabelText(i18n.t('providers.templates.name')), '登录验证码中文')
    await user.type(screen.getByLabelText(i18n.t('providers.templates.code')), 'SMS_100')
    const providerParam = screen.getByPlaceholderText(i18n.t('providers.templates.providerParamPlaceholder'))
    await user.clear(providerParam)
    await user.type(providerParam, 'code1')
    await user.click(screen.getByRole('button', { name: i18n.t('common.save') }))

    await waitFor(() => expect(providerApi.createTemplate).toHaveBeenCalledTimes(1))
    expect(providerApi.createTemplate).toHaveBeenCalledWith(7, {
      name: '登录验证码中文',
      scene: 'login_code',
      template_code: 'SMS_100',
      template_params: [{ key: 'code1', source: 'code' }],
      subject: undefined,
      content: undefined,
      is_default: false,
    })
  })

  it('邮件模板只展示当前场景占位符，并提交富文本正文', async () => {
    vi.mocked(providerApi.get).mockResolvedValue(EMAIL_PROVIDER)
    vi.mocked(providerApi.templates).mockResolvedValue({ items: [] })
    const user = userEvent.setup()
    renderPage()

    const addButtons = await screen.findAllByRole('button', { name: i18n.t('providers.templates.add') })
    await user.click(addButtons[0])

    await user.type(screen.getByLabelText(i18n.t('providers.templates.name')), '邮件登录验证码')
    const editor = screen.getByLabelText('rich-text')
    fireEvent.change(editor, { target: { value: '<p>请使用验证码登录</p>' } })
    const placeholderButtons = screen.getAllByRole('button', { name: /code/ })
    await user.click(placeholderButtons[placeholderButtons.length - 1])
    expect((editor as HTMLTextAreaElement).value).toContain('{{code}}')
    expect(screen.queryByRole('button', { name: /link/ })).toBeNull()

    await user.click(screen.getByRole('button', { name: i18n.t('common.save') }))
    await waitFor(() => expect(providerApi.createTemplate).toHaveBeenCalledTimes(1))
    const payload = vi.mocked(providerApi.createTemplate).mock.calls[0][1]
    expect(payload.scene).toBe('login_code')
    expect(payload.content).toContain('{{code}}')
    expect(payload.template_params).toEqual([])
  })

  it('兼容后端历史响应：邮件模板 template_params 为 null 时可正常编辑', async () => {
    vi.mocked(providerApi.get).mockResolvedValue(EMAIL_PROVIDER)
    vi.mocked(providerApi.templates).mockResolvedValue({
      items: [
        {
          ...SMS_TEMPLATE,
          id: 12,
          provider_id: 8,
          channel: 'email',
          subject: '邮件验证码',
          content: '<p>您的验证码是 {{code}}</p>',
          template_code: '',
          template_params: null,
        } as unknown as ProviderTemplate,
      ],
    })
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('登录验证码')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: i18n.t('common.edit') }))
    expect(await screen.findByText(i18n.t('providers.templates.editTitle'))).toBeInTheDocument()
  })
})
