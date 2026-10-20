import { StrictMode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { AuthProvider } from './auth-context'
import LoginPage from './login-page'

const mocks = vi.hoisted(() => ({ navigateDocument: vi.fn() }))

vi.mock('@/shared/lib/navigation', () => ({ navigateDocument: mocks.navigateDocument }))

// 引导接口的打点：用最简响应让页面走完"应用驱动登录页"的初始化
// 登录请求体打点（断言"只传主键、不传标识"）
let lastLoginPayload: Record<string, unknown> | null = null
let lastMfaVerifyPayload: Record<string, unknown> | null = null
// 验证码通道打点：应用驱动入口的发码与登录都必须携带应用上下文（client_id）
let lastSendCodePayload: Record<string, unknown> | null = null
let lastCodeLoginPayload: Record<string, unknown> | null = null
// 第三方 SSO 发起参数：必须保留上游 OIDC 的续授权地址
let lastOAuthAuthorizeArgs: unknown[] | null = null
// 应用的登录方式与提供方白名单（默认 = 只允许密码、没配任何第三方提供方）
let appSigninMethods: { name: string }[] = [{ name: 'Password' }]
let appProviders: { name: string; canSignIn: boolean }[] = []
// 已登录会话 / 应用准入预检（用于"当前用户无权访问该应用"的提醒页用例）
let authenticated = false
let appAccessAllowed = true

/** 打开"第三方登录"：应用把 Provider 列为登录方式并给出具体提供方 */
function enableThirdPartySignin() {
  appSigninMethods = [{ name: 'Password' }, { name: 'Provider' }]
  appProviders = [{ name: 'corp-dingtalk', canSignIn: true }]
}

/** 还原为默认（仅密码） */
function resetSigninMethods() {
  appSigninMethods = [{ name: 'Password' }]
  appProviders = []
}

// 是否让登录返回"登录方式要求绑定 MFA"（用于验证绑定页的二维码）
let forceMfaEnroll = false
// 是否让登录返回"需要第二因子"（用于验证独立第二屏）
let requireMfaCode = false

const calls = {
  status: 0,
  institutions: 0,
  providers: 0,
  appInfoByClientId: 0,
}

vi.mock('@/shared/api/auth', () => ({
  authApi: {
    status: async () => {
      calls.status += 1
      return { authenticated, institution_id: authenticated ? 2 : 0, permissions: [] }
    },
    logout: async () => undefined,
    institutions: async () => {
      calls.institutions += 1
      return { institutions: [] }
    },
    providers: async () => {
      calls.providers += 1
      return {
        providers: [
          { name: 'corp-dingtalk', display_name: '钉钉登录', type: 'dingtalk', icon: 'dingtalk' },
          // 老数据没有 type/icon：按钮仍要能渲染（退化为占位图形 + 名称提示）
          { name: 'legacy-sso', display_name: '老自建 IdP' },
        ],
      }
    },
    appAccess: async () => ({
      allowed: appAccessAllowed,
      reason: appAccessAllowed ? undefined : 'app_access_denied',
      application_code: 'net-bird',
      application_name: 'Net Bird',
    }),
    appInfoByClientId: async () => {
      calls.appInfoByClientId += 1
      return {
        institution: '测试',
        institution_id: 2,
        code: 'net-bird',
        name: 'Net Bird',
        signin_methods: appSigninMethods,
        self_signup_enabled: false,
        signup_auto_signin: false,
        magic_link_ttl_seconds: 600,
        captcha_required: false,
        sms_providers: [],
        email_providers: [],
        providers: appProviders,
        signup_items: [],
      }
    },
    captcha: async () => ({ required: false }),
    oauthAuthorize: async (...args: unknown[]) => {
      lastOAuthAuthorizeArgs = args
      return { authorization_url: 'https://idp.example.com/authorize' }
    },
    sendCode: async (body: Record<string, unknown>) => {
      lastSendCodePayload = body
      return { status: 'sent' }
    },
    codeLogin: async (body: Record<string, unknown>) => {
      lastCodeLoginPayload = body
      return {
        user: { institution_id: 2, name: 'testlogin' },
        session: { id: 's2', expires_at: 0 },
      }
    },
    login: async (body: Record<string, unknown>) => {
      lastLoginPayload = body
      if (forceMfaEnroll) {
        return { mfa: { required: true, enroll: true, type: 'totp' } }
      }
      if (requireMfaCode && !body.mfa_code) {
        return { mfa: { required: true, type: 'totp', remember_hours: 12 } }
      }
      return {
        user: { institution_id: 2, name: 'testlogin' },
        session: { id: 's1', expires_at: 0 },
      }
    },
    mfaOptions: async () => ({
      required: true,
      can_skip: false,
      providers: [{ id: 5, type: 'totp', name: 'default-totp', display_name: '虚拟 MFA' }],
      bound: requireMfaCode
        ? [{ provider_id: 5, type: 'totp', name: 'default-totp', display_name: '虚拟 MFA' }]
        : [],
      remember_hours: 12,
    }),
    mfaEnroll: async (providerID: number) => ({
      provider_id: providerID,
      type: 'totp',
      secret: 'JBSWY3DPEHPK3PXP',
      url: 'otpauth://totp/Polaris:testlogin@ceshi?secret=JBSWY3DPEHPK3PXP&issuer=Polaris',
    }),
    mfaResendEnroll: async () => ({ status: 'sent' }),
    mfaEnrollVerify: async () => ({ recovery_codes: [] }),
    mfaVerify: async (providerID: number, code: string, remember: boolean) => {
      lastMfaVerifyPayload = { provider_id: providerID, code, remember }
      return { status: 'ok' }
    },
    mfaSkip: async () => ({ status: 'ok' }),
    mfaDisable: async () => ({ status: 'ok' }),
    appInfo: async () => {
      throw new Error('未指定组织时不应调用 appInfo')
    },
  },
}))

/**
 * 回归：打开登录页时，每个引导接口只能请求一次。
 *
 * 历史问题：`loadAppInfo` 会把解析出的组织写回 state，而它自己又依赖这两个 state，
 * 于是"依赖变化 → 再请求一轮"，叠加 StrictMode 的双调用后，
 * Network 面板里能看到 status ×4、providers/app-info ×3、institutions ×2。
 */
describe('登录页引导请求次数', () => {
  it('StrictMode 下每个引导接口只请求一次', async () => {
    render(
      <StrictMode>
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>
      </StrictMode>,
    )

    // 等应用名渲染出来（说明 app-info 已回填）
    expect(await screen.findByText('Net Bird')).toBeInTheDocument()

    expect(calls).toEqual({
      status: 1,
      institutions: 1,
      providers: 1,
      appInfoByClientId: 1,
    })
  })
})

/**
 * 回归：公开面请求的组织参数**只传一个**——有主键就只传 `institution_id`，
 * 不再同时附带 `institution`（同请求两个来源会产生"谁是权威"的歧义）。
 */
describe('登录请求的组织参数', () => {
  it('应用驱动登录只传 institution_id', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
          <AuthProvider>
            <LoginPage />
          </AuthProvider>
        </MemoryRouter>
      </I18nextProvider>,
    )
    expect(await screen.findByText('Net Bird')).toBeInTheDocument()

    // 标签词条按当前界面语言取（测试环境为英文）
    await userEvent.type(screen.getByLabelText(i18n.t('auth.username')), 'testlogin')
    await userEvent.type(screen.getByLabelText(i18n.t('auth.password')), 'test123456')
    await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.signIn') }))

    expect(lastLoginPayload).not.toBeNull()
    expect(lastLoginPayload).toMatchObject({ institution_id: 2, username: 'testlogin' })
    expect(lastLoginPayload).not.toHaveProperty('institution')
    // 应用驱动入口必须带上应用上下文（client_id）：否则后端按"组织默认应用"解析，
    // 应用的准入判定与登录方式都会落空——典型的"配了不生效"
    expect(lastLoginPayload).toMatchObject({ client_id: 'f555cd666d51f1066d6b' })
  })

  it('应用驱动登录的验证码通道同样携带 client_id（发码 + 登录）', async () => {
    // 打开验证码通道（应用把 EmailCode 列为登录方式）
    appSigninMethods = [{ name: 'Password' }, { name: 'EmailCode' }]
    lastSendCodePayload = null
    lastCodeLoginPayload = null
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )
      expect(await screen.findByText('Net Bird')).toBeInTheDocument()

      await userEvent.click(screen.getByRole('tab', { name: i18n.t('auth.codeTab') }))
      await userEvent.type(screen.getByLabelText(i18n.t('auth.codeTarget')), 'u@example.com')
      await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.sendCode') }))
      await waitFor(() => expect(lastSendCodePayload).not.toBeNull())
      expect(lastSendCodePayload).toMatchObject({ client_id: 'f555cd666d51f1066d6b' })

      await userEvent.type(screen.getByLabelText(i18n.t('auth.codeValue')), '123456')
      await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.codeSignIn') }))
      await waitFor(() => expect(lastCodeLoginPayload).not.toBeNull())
      expect(lastCodeLoginPayload).toMatchObject({ client_id: 'f555cd666d51f1066d6b', code: '123456' })
    } finally {
      resetSigninMethods()
    }
  })
})

/**
 * 回归：强制绑定二次验证时，必须给**二维码**（而不是只给一串密钥/链接），
 * 手动密钥与 otpauth 链接作为兜底保留。
 */
describe('二次验证绑定页', () => {
  it('给出二维码 + 手动兜底密钥', async () => {
    forceMfaEnroll = true
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )
      expect(await screen.findByText('Net Bird')).toBeInTheDocument()

      await userEvent.type(screen.getByLabelText(i18n.t('auth.username')), 'testlogin')
      await userEvent.type(screen.getByLabelText(i18n.t('auth.password')), 'test123456')
      await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.signIn') }))

      // 独立绑定屏：标题 + 二维码（qrcode.react 渲染成 168×168 的 svg，用尺寸精确定位避免误命中图标）
      expect(await screen.findByText(i18n.t('auth.enrollTitle'))).toBeInTheDocument()
      const qr = document.querySelector('svg[height="168"][width="168"]')
      expect(qr).not.toBeNull()
      expect(document.querySelectorAll('[data-slot="input-otp-slot"]')).toHaveLength(6)
      // 手动密钥收在"无法扫码？"里：默认不显示，点开才出现（一屏只留扫码这一条主路径）
      expect(screen.queryByText('JBSWY3DPEHPK3PXP')).toBeNull()
      await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.enrollManual') }))
      expect(screen.getByText('JBSWY3DPEHPK3PXP')).toBeInTheDocument()
      expect(screen.getByText(/^otpauth:\/\/totp\//)).toBeInTheDocument()
    } finally {
      forceMfaEnroll = false
    }
  })
})

/**
 * 回归：需要第二因子时，必须**切换到独立一屏**（对齐 Casdoor `MfaVerify`），
 * 而不是把「动态码」塞进凭据表单里让用户把账号密码再提交一遍；
 * 且第二因子屏不得再显示第三方登录入口（避免"可以绕开二次验证"的错觉）。
 */
describe('登录第二因子', () => {
  it('切屏要求动态码，填满 6 位自动提交并带上保存的凭据', async () => {
    requireMfaCode = true
    enableThirdPartySignin() // 让"第二因子屏不显示第三方入口"这条断言有实际意义
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )
      expect(await screen.findByText('Net Bird')).toBeInTheDocument()

      await userEvent.type(screen.getByLabelText(i18n.t('auth.username')), 'ceshi')
      await userEvent.type(screen.getByLabelText(i18n.t('auth.password')), 'test123456')
      await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.signIn') }))

      // 切屏：凭据字段消失，出现独立的第二因子屏
      expect(await screen.findByText(i18n.t('auth.mfaStepTitle'))).toBeInTheDocument()
      expect(screen.queryByLabelText(i18n.t('auth.password'))).toBeNull()
      const codeInput = screen.getByLabelText(i18n.t('auth.mfaCode'))
      expect(document.activeElement).toBe(codeInput) // 自动聚焦，用户直接输码
      // 一格一格的分段输入（6 格、彼此独立），不是单个输入框
      expect(document.querySelectorAll('[data-slot="input-otp-slot"]')).toHaveLength(6)
      // 第二因子屏不显示第三方登录入口
      expect(screen.queryByRole('button', { name: '钉钉登录' })).toBeNull()
      expect(screen.queryByText(i18n.t('auth.or'))).toBeNull()
      expect(screen.getByText(i18n.t('auth.rememberDevice', { hours: 12 }))).toBeInTheDocument()

      // 填满 6 位 → 自动提交，且请求里带上保存的凭据与动态码
      await userEvent.type(codeInput, '123456')
      await waitFor(() => {
        expect(lastMfaVerifyPayload).toMatchObject({ provider_id: 5, code: '123456' })
      })
    } finally {
      requireMfaCode = false
      resetSigninMethods()
    }
  })

  it('「返回上一步」回到凭据屏并恢复第三方登录入口', async () => {
    requireMfaCode = true
    enableThirdPartySignin()
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )
      expect(await screen.findByText('Net Bird')).toBeInTheDocument()
      await userEvent.type(screen.getByLabelText(i18n.t('auth.username')), 'ceshi')
      await userEvent.type(screen.getByLabelText(i18n.t('auth.password')), 'test123456')
      await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.signIn') }))
      expect(await screen.findByText(i18n.t('auth.mfaStepTitle'))).toBeInTheDocument()

      await userEvent.click(screen.getByRole('button', { name: new RegExp(i18n.t('auth.mfaBack')) }))

      // 回到凭据屏：密码框与第三方登录入口都回来了
      expect(await screen.findByLabelText(i18n.t('auth.password'))).toBeInTheDocument()
      expect(await screen.findByRole('button', { name: '钉钉登录' })).toBeInTheDocument()
    } finally {
      requireMfaCode = false
      resetSigninMethods()
    }
  })
})

/**
 * 回归（真实缺陷）：应用**没有配置**第三方提供方时，登录页不得出现任何第三方入口。
 *
 * 现场：应用 net-bird 只启用密码登录、providers 为空，登录页却出现「钉钉登录」——
 * 因为读取侧把"空列表"当成了"全都要"，与写入侧 validateApplication
 * （启用第三方登录必须带具体提供方）自相矛盾。
 */
/**
 * 回归（真实反馈）：必填项为空时不再弹浏览器原生气泡（"Please fill out this field."），
 * 改为在输入框上加红边框 + 红色光晕，并给出内联提示。
 */
describe('登录表单必填校验', () => {
  it('空表单提交：标红账号与密码、显示内联提示，且不发登录请求', async () => {
    lastLoginPayload = null
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
          <AuthProvider>
            <LoginPage />
          </AuthProvider>
        </MemoryRouter>
      </I18nextProvider>,
    )
    expect(await screen.findByText('Net Bird')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.signIn') }))

    expect(document.getElementById('username')).toHaveAttribute('aria-invalid', 'true')
    expect(document.getElementById('password')).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getAllByText(i18n.t('auth.fieldRequired'))).toHaveLength(2)
    expect(lastLoginPayload).toBeNull()
  })

  it('开始输入后立刻撤销该字段的红框', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
          <AuthProvider>
            <LoginPage />
          </AuthProvider>
        </MemoryRouter>
      </I18nextProvider>,
    )
    expect(await screen.findByText('Net Bird')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: i18n.t('auth.signIn') }))
    expect(document.getElementById('password')).toHaveAttribute('aria-invalid', 'true')

    await userEvent.type(document.getElementById('password')!, 'secret')
    expect(document.getElementById('password')).not.toHaveAttribute('aria-invalid')
  })
})

/**
 * 回归（真实缺陷）：应用**没有配置**第三方提供方时，登录页不得出现任何第三方入口。
 *
 * 现场：应用 net-bird 只启用密码登录、providers 为空，登录页却出现「钉钉登录」——
 * 因为读取侧把"空列表"当成了"全都要"，与写入侧 validateApplication
 * （启用第三方登录必须带具体提供方）自相矛盾。
 */
describe('第三方登录入口', () => {
  it('应用未配置提供方时，一个第三方入口都不显示', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
          <AuthProvider>
            <LoginPage />
          </AuthProvider>
        </MemoryRouter>
      </I18nextProvider>,
    )
    expect(await screen.findByText('Net Bird')).toBeInTheDocument()

    // /auth/providers 仍返回了组织级启用的提供方（corp-dingtalk / legacy-sso），
    // 但应用侧没有授权任何提供方 → 页面不得渲染入口，也不显示"或"分隔
    expect(screen.queryByRole('button', { name: '钉钉登录' })).toBeNull()
    expect(screen.queryByRole('button', { name: '老自建 IdP' })).toBeNull()
    expect(screen.queryByText(i18n.t('auth.or'))).toBeNull()
  })

  it('第三方登录会把上游 OIDC 续授权地址写入 SSO from', async () => {
    enableThirdPartySignin()
    lastOAuthAuthorizeArgs = null
    mocks.navigateDocument.mockReset()
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter
            initialEntries={[
              '/login?client_id=f555cd666d51f1066d6b&redirect_uri=https%3A%2F%2Fnetbird.example.com%2Fcallback&response_type=code&state=upstream-state',
            ]}
          >
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )
      expect(await screen.findByText('Net Bird')).toBeInTheDocument()
      await userEvent.click(await screen.findByRole('button', { name: '钉钉登录' }))

      await waitFor(() => expect(lastOAuthAuthorizeArgs).not.toBeNull())
      expect(lastOAuthAuthorizeArgs?.[0]).toBe('corp-dingtalk')
      const target = new URL(String(lastOAuthAuthorizeArgs?.[1]), 'https://polaris.example.com')
      expect(target.pathname).toBe('/oauth/authorize')
      expect(target.searchParams.get('client_id')).toBe('f555cd666d51f1066d6b')
      expect(target.searchParams.get('redirect_uri')).toBe('https://netbird.example.com/callback')
      expect(target.searchParams.get('state')).toBe('upstream-state')
      expect(mocks.navigateDocument).toHaveBeenCalledWith('https://idp.example.com/authorize')
    } finally {
      resetSigninMethods()
    }
  })

  it('应用配置了提供方时，只显示白名单内 canSignIn 的入口', async () => {
    enableThirdPartySignin()
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )
      expect(await screen.findByText('Net Bird')).toBeInTheDocument()
      expect(await screen.findByRole('button', { name: '钉钉登录' })).toBeInTheDocument()
      // 白名单只有 corp-dingtalk：legacy-sso 不显示
      expect(screen.queryByRole('button', { name: '老自建 IdP' })).toBeNull()
    } finally {
      resetSigninMethods()
    }
  })

  it('图标悬浮时展示身份提供商的名称', async () => {
    enableThirdPartySignin()
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/login?client_id=f555cd666d51f1066d6b']}>
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )
      expect(await screen.findByText('Net Bird')).toBeInTheDocument()
      await userEvent.hover(await screen.findByRole('button', { name: '钉钉登录' }))

      await waitFor(() => expect(screen.getByRole('tooltip')).toHaveTextContent('钉钉登录'))
    } finally {
      resetSigninMethods()
    }
  })
})

/**
 * 回归（2026-10）：已登录用户从应用入口回到登录页、但没有该应用授权时，
 * 不能把浏览器送到 /oauth/authorize 后展示一段 JSON；应停留在本地提醒页，并可返回用户门户。
 */
describe('应用授权提醒', () => {
  it('未授权时展示提醒页与返回门户按钮，不再自动续走 authorize', async () => {
    authenticated = true
    appAccessAllowed = false
    try {
      render(
        <I18nextProvider i18n={i18n}>
          <MemoryRouter
            initialEntries={[
              '/login?client_id=f555cd666d51f1066d6b&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback',
            ]}
          >
            <AuthProvider>
              <LoginPage />
            </AuthProvider>
          </MemoryRouter>
        </I18nextProvider>,
      )

      expect(await screen.findByText(i18n.t('auth.appAccessDeniedTitle'))).toBeInTheDocument()
      expect(screen.getByRole('button', { name: i18n.t('auth.backToPortal') })).toBeInTheDocument()
    } finally {
      authenticated = false
      appAccessAllowed = true
    }
  })
})
