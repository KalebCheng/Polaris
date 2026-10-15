import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import zh from '@/shared/i18n/locales/zh.json'
import ProviderEditPage, { toPayload } from './provider-edit-page'
import type { ProviderTypeInfo } from '@/shared/api/types'

// 当前组织由工作区上下文提供
vi.mock('../institution-context', () => ({
  useWorkspaceInstitution: () => 'acme',
  useWorkspaceInstitutionId: () => '1',
}))

const OIDC_TYPE: ProviderTypeInfo = {
  key: 'oidc',
  name: '标准 OIDC',
  description: 'desc',
  icon: 'oidc',
  category: 'oauth',
  capabilities: { signin: true, notify: false, captcha: false, storage: false, test: true },
  fields: [
    { key: 'name', label: '标识', group: 'basic', type: 'text', required: true, immutable: true },
    { key: 'display_name', label: '名称', group: 'basic', type: 'text' },
    { key: 'state', label: '状态', group: 'basic', type: 'select', options: [{ value: 'active', label: '启用' }] },
    { key: 'client_id', label: '客户端 ID', group: 'connection', type: 'text', required: true },
    { key: 'client_secret', label: '客户端密钥', group: 'connection', type: 'password', required: true, secret: true },
    { key: 'issuer_url', label: 'Issuer 地址', group: 'connection', type: 'text', required: true },
    { key: 'enable_pkce', label: '启用 PKCE', group: 'advanced', type: 'switch', default: 'true' },
    { key: 'auto_create', label: '自动创建本地账号', group: 'advanced', type: 'switch', policy: true, default: 'true' },
    {
      key: 'allowed_domains',
      label: '允许的邮箱域名',
      group: 'advanced',
      type: 'tags',
      policy: true,
    },
  ],
}

// 目录是类型与字段 schema 的唯一来源
vi.mock('../api', () => ({
  providerApi: {
    catalog: async () => ({
      categories: [
        {
          key: 'oauth',
          name: '外部身份提供方',
          description: 'desc',
          types: [
            {
              key: 'oidc',
              name: '标准 OIDC',
              description: 'desc',
              icon: 'oidc',
              category: 'oauth',
              capabilities: { signin: true, notify: false, captcha: false, storage: false, test: true },
              fields: [
                { key: 'name', label: '标识', group: 'basic', type: 'text', required: true, immutable: true },
    { key: 'display_name', label: '名称', group: 'basic', type: 'text' },
                { key: 'display_name', label: '名称', group: 'basic', type: 'text' },
                { key: 'state', label: '状态', group: 'basic', type: 'select' },
                { key: 'client_id', label: '客户端 ID', group: 'connection', type: 'text', required: true },
                { key: 'client_secret', label: '客户端密钥', group: 'connection', type: 'password', secret: true },
                { key: 'issuer_url', label: 'Issuer 地址', group: 'connection', type: 'text', required: true },
                { key: 'enable_pkce', label: '启用 PKCE', group: 'advanced', type: 'switch', default: 'true' },
                { key: 'auto_create', label: '自动创建本地账号', group: 'advanced', type: 'switch', policy: true, default: 'true' },
                { key: 'allowed_domains', label: '允许的邮箱域名', group: 'advanced', type: 'tags', policy: true },
              ],
            },
          ],
        },
        { key: 'email', name: '邮件', description: 'desc', types: [] },
        {
          key: 'sms',
          name: '短信',
          description: 'desc',
          types: [
            {
              key: 'aliyun',
              name: '阿里云短信',
              description: 'desc',
              icon: 'aliyun',
              category: 'sms',
              capabilities: { signin: false, notify: true, captcha: false, storage: false, test: true },
              fields: [
                { key: 'name', label: '标识', group: 'basic', type: 'text', required: true, immutable: true },
    { key: 'display_name', label: '名称', group: 'basic', type: 'text' },
                { key: 'display_name', label: '名称', group: 'basic', type: 'text' },
                { key: 'client_id', label: 'AccessKey ID', group: 'connection', type: 'text', required: true },
                { key: 'client_secret', label: 'AccessKey Secret', group: 'connection', type: 'password', secret: true },
                { key: 'sign_name', label: '短信签名', group: 'connection', type: 'text' },
                { key: 'template_code', label: '短信模板编号', group: 'connection', type: 'text' },
                { key: 'endpoint', label: '接口地址', group: 'advanced', type: 'text', hint: '留空使用阿里云默认地址' },
                {
                  key: 'template_params',
                  label: '模板变量',
                  group: 'advanced',
                  type: 'multiselect',
                  default: 'code',
                  options: [
                    { value: 'code', label: '验证码' },
                    { value: 'minutes', label: '有效分钟数' },
                  ],
                },
                { key: 'region_id', label: '区域', group: 'advanced', type: 'text', hint: '留空使用 cn-hangzhou' },
                { key: 'receiver', label: '自检手机号', group: 'advanced', type: 'text' },
              ],
            },
          ],
        },
      ],
    }),
    create: vi.fn(),
    update: vi.fn(),
    get: async () => ({
      id: 42,
      institution_id: 1,
      name: 'corp-sso',
      display_name: '企业单点登录',
      category: 'oauth',
      type: 'oidc',
      state: 'active',
      config: { issuer_url: 'https://idp.example.com', client_id: 'polaris', enable_pkce: true },
      policy: { auto_create: true },
      has_secret: true,
      created_at: '',
      updated_at: '',
    }),
    test: vi.fn(),
  },
  platformApi: { oauthInfo: async () => ({ callback_url: 'https://iam.example.com/oauth/callback', homepage_url: 'https://iam.example.com' }) },
}))

function renderPage(entry = '/institutions/acme/providers/new?category=oauth&type=oidc') {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route path="/institutions/:inst/providers/new" element={<ProviderEditPage />} />
          <Route path="/institutions/:inst/providers/:id" element={<ProviderEditPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

describe('ProviderEditPage', () => {
  it('字段词条必须是字符串（回归：曾把 providers.field.x 写成对象，界面显示 i18next 报错原文）', () => {
    // 断言字面量，而不是再次调用 i18n.t —— 坏掉时两处会拿到同一条报错字符串而"自证通过"
    expect(i18n.t('providers.field.name')).toBe('Identifier')
    expect(i18n.t('providers.field.client_id')).toBe('Client ID')
    // 类型专属词条按类型解析（同一列名在不同类型下含义不同）
    expect(i18n.t('providers.field.aliyun.endpoint')).toBe('API endpoint')
    expect(i18n.t('providers.field.local.endpoint')).toBe('Storage root path')
    expect(i18n.t('providers.field.nameHint')).toBe(
      'Unique within the organization; cannot be changed after creation',
    )
    // 中文词条同样必须是字符串（界面语言可切换）
    expect(zh.providers.field.name).toBe('标识')
    expect(zh.providers.field.client_id).toBe('客户端 ID')
    expect(zh.providers.field.aliyun.endpoint).toBe('接口地址')
  })

  it('字段只出现在所属步骤（基本信息 → 连接配置 → 高级与策略）', async () => {
    renderPage()
    expect(await screen.findByText('标准 OIDC')).toBeInTheDocument()

    // 第一步：只有基本信息
    expect(screen.getByText('Identifier')).toBeInTheDocument()
    expect(screen.queryByText('Client ID')).toBeNull()
    expect(screen.queryByText('Auto-create local accounts')).toBeNull()

    const nav = screen.getByRole('navigation', { name: i18n.t('wizard.steps') })
    const buttons = within(nav).getAllByRole('button')

    // 第二步：连接配置（凭据与端点）——类型卡片只在第一步出现，这里必须消失
    await userEvent.click(buttons[1])
    expect(screen.queryByText('标准 OIDC')).toBeNull()
    expect(screen.queryByRole('button', { name: i18n.t('providers.changeType') })).toBeNull()
    expect(screen.getByText('Client ID')).toBeInTheDocument()
    expect(screen.getByText('Issuer URL')).toBeInTheDocument()
    expect(screen.queryByText('Auto-create local accounts')).toBeNull()

    // 第三步：高级与策略（PKCE + SSO 信任策略），并展示开发信息；类型卡片依旧不出现
    await userEvent.click(buttons[2])
    expect(screen.getByText('Enable PKCE')).toBeInTheDocument()
    expect(screen.getByText('Auto-create local accounts')).toBeInTheDocument()
    expect(screen.getByText(i18n.t('providers.devInfo'))).toBeInTheDocument()
    expect(screen.queryByText('标准 OIDC')).toBeNull()

    // 回到第一步：类型卡片与「更换类型」应重新出现（回归：曾写在步骤条件之外，每步都重复渲染）
    await userEvent.click(buttons[0])
    expect(screen.getByText('标准 OIDC')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: i18n.t('providers.changeType') })).toBeInTheDocument()
  })

  it('类型专属标签：阿里云短信的高级步显示「接口地址 / 区域 / 自检手机号」，不是通用的「端点」', async () => {
    renderPage('/institutions/acme/providers/new?category=sms&type=aliyun')
    expect(await screen.findByText('阿里云短信')).toBeInTheDocument()

    const nav = screen.getByRole('navigation', { name: i18n.t('wizard.steps') })
    const buttons = within(nav).getAllByRole('button')
    await userEvent.click(buttons[2])

    // 关键：这一列在阿里云短信里叫「接口地址」，而不是把存储/网关含义拼进来的通用标签
    expect(screen.getByText('API endpoint')).toBeInTheDocument()
    expect(screen.getByText('Defaults to cn-hangzhou')).toBeInTheDocument()
    // 模板变量是"受限多选"：只能从内置变量集里选（阿里云对变量名多一个少一个都报错）
    expect(screen.getByText('Template variables')).toBeInTheDocument()
    expect(screen.queryByText('Endpoint')).toBeNull()
    expect(screen.queryByText('Storage root path')).toBeNull()
  })

  it('编辑路径：直接进入向导而不是"重新选类型"（URL 只有 id，类型来自记录）', async () => {
    renderPage('/institutions/acme/providers/42')

    // 首帧（记录尚未到位）也不得出现"选择类型"页——编辑态的类型来自记录，不来自 URL
    expect(screen.queryByRole('button', { name: i18n.t('providers.addThisType') })).toBeNull()

    // 回归：曾因"先等 typeInfo 再取记录"而死锁，编辑会停在类型选择页
    expect(await screen.findByText('标准 OIDC')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: i18n.t('providers.addThisType') })).toBeNull()
    expect(screen.getByRole('navigation', { name: i18n.t('wizard.steps') })).toBeInTheDocument()

    // 编辑态：标识不可改、名称回填、类型卡片只作标识展示（无「更换类型」）
    const name = screen.getByLabelText(/Identifier/) as HTMLInputElement
    expect(name.value).toBe('corp-sso')
    expect(name.disabled).toBe(true)
    // 名称按标签查（英文界面下 display_name 的词条是 "Name"）
    const labels = Array.from(document.querySelectorAll('label')).map((el) => el.textContent)
    expect(labels).toContain('Name')
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('企业单点登录')
    expect(screen.queryByRole('button', { name: i18n.t('providers.changeType') })).toBeNull()
  })

  it('未带类型时先进入整页目录（不使用弹窗）', async () => {
    renderPage('/institutions/acme/providers/new')
    expect(await screen.findByText('标准 OIDC')).toBeInTheDocument()
    // 每个类型一张卡片，各带一个「添加」（此处只关心目录页形态）
    expect(screen.getAllByRole('button', { name: i18n.t('providers.addThisType') }).length).toBeGreaterThan(0)
    // 目录页不应出现向导步骤条
    expect(screen.queryByRole('navigation', { name: i18n.t('wizard.steps') })).toBeNull()
  })
})

describe('toPayload', () => {
  it('短信模板变量作为数组进 config（按目录声明装配）', () => {
    const aliyun = {
      key: 'aliyun',
      name: '阿里云短信',
      description: 'd',
      icon: 'aliyun',
      category: 'sms',
      capabilities: { signin: false, notify: true, captcha: false, storage: false, test: true },
      fields: [
        { key: 'name', label: '标识', group: 'basic', type: 'text', required: true, immutable: true },
    { key: 'display_name', label: '名称', group: 'basic', type: 'text' },
        { key: 'template_code', label: '短信模板编号', group: 'connection', type: 'text' },
        {
          key: 'template_params',
          label: '模板变量',
          group: 'advanced',
          type: 'multiselect',
          options: [
            { value: 'code', label: '验证码' },
            { value: 'minutes', label: '有效分钟数' },
          ],
        },
      ],
    } as unknown as ProviderTypeInfo
    const payload = toPayload(aliyun, { name: 'sms-1', template_code: 'SMS_1', template_params: ['code', 'minutes'] })
    expect(payload.config).toEqual({ template_code: 'SMS_1', template_params: ['code', 'minutes'] })
    expect(payload.secrets).toEqual({})
  })

  it('按目录声明分桶：敏感进 secrets、策略进 policy、其余进 config', () => {
    const payload = toPayload(OIDC_TYPE, {
      name: 'corp-sso',
      display_name: '企业单点登录',
      state: 'active',
      client_id: 'polaris',
      client_secret: 's3cr3t',
      issuer_url: 'https://idp.example.com',
      enable_pkce: true,
      auto_create: true,
      allowed_domains: ['corp.example.com'],
    })
    expect(payload.category).toBe('oauth')
    expect(payload.type).toBe('oidc')
    expect(payload.secrets).toEqual({ client_secret: 's3cr3t' })
    expect(payload.config).toEqual({
      client_id: 'polaris',
      issuer_url: 'https://idp.example.com',
      enable_pkce: true,
    })
    expect(payload.policy).toEqual({ auto_create: true, allowed_domains: ['corp.example.com'] })
    // 敏感字段绝不能出现在 config 里（后端会直接拒绝）
    expect(payload.config).not.toHaveProperty('client_secret')
  })

  it('编辑态密钥留空 = 不提交该键（保持原值）', () => {
    const payload = toPayload(OIDC_TYPE, { name: 'corp-sso', client_id: 'polaris', client_secret: '' })
    expect(payload.secrets).toEqual({})
  })
})
