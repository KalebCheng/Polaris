import { useTranslation } from 'react-i18next'
import { Languages, Moon } from 'lucide-react'
import { BrandLockup } from '@/shared/components/brand-lockup'
import { SigninMethodIcon } from '@/shared/components/signin-method-icon'
import { Button, Input, Label } from '@/shared/ui'

interface LoginPagePreviewProps {
  /** 组织/应用 Logo（同时用于页眉品牌与卡片图标） */
  logo?: string
  /** 卡片大标题（组织的显示名称 / 应用的名称） */
  displayName?: string
  /** 兜底名称（组织 / 应用的标识） */
  name?: string
  /** 欢迎语（组织描述；未填则不显示） */
  description?: string
  /** 登录卡片图标：当前应用的 icon（未配置时回退组织 Logo） */
  cardLogo?: string
  /** 品牌主色（应用主题色；未配置回退主题主色） */
  accentColor?: string
  /** 已启用的登录方式（顺序即展示顺序）；默认仅密码 */
  methods?: string[]
  /** 预览页签：登录页 / 注册页 */
  mode?: 'signin' | 'signup'
  /** 是否展示「注册」入口 */
  allowSignUp?: boolean
}

/**
 * 整页登录页 / 注册页预览：按真实页面的版式（页眉品牌 + 居中卡片 + 页脚）等比还原。
 * 整体 `pointer-events-none`，纯展示、不触发真实交互（登录/注册页签由调用方通过 mode 控制）。
 */
export function LoginPagePreview({
  logo,
  displayName,
  name,
  description,
  cardLogo,
  accentColor,
  methods = ['Password'],
  mode = 'signin',
  allowSignUp = false,
}: LoginPagePreviewProps) {
  const { t } = useTranslation()
  const heading = displayName?.trim() || name?.trim() || t('app.name')
  const accent = accentColor?.trim() || 'var(--primary)'
  const hasPassword = methods.includes('Password')
  const hasCode = methods.includes('EmailCode') || methods.includes('SMSCode')
  const hasMagic = methods.includes('MagicLink')
  const hasProvider = methods.includes('Provider')

  return (
    <div className="bg-background pointer-events-none relative flex h-[min(62vh,560px)] flex-col overflow-hidden select-none">
      {/* 页眉：品牌标识 ＋ 语言 / 主题切换（静态示意） */}
      <header className="flex items-center justify-between gap-4 px-6 py-5 sm:px-8">
        <div className="flex items-center gap-2.5">
          <BrandLockup src={logo} name={heading} size="sm" />
        </div>
        <div className="text-muted-foreground flex items-center gap-3">
          <Moon className="size-4" aria-hidden="true" />
          <Languages className="size-4" aria-hidden="true" />
        </div>
      </header>

      {/* 主体：居中的卡片 */}
      <main className="flex flex-1 items-center justify-center px-6 py-4">
        <div className="surface-strong w-full max-w-sm rounded-2xl p-7">
          <header className="space-y-2 text-center">
            {(cardLogo ?? logo)?.trim() ? (
              <img
                src={(cardLogo ?? logo)!.trim()}
                alt=""
                className="mx-auto mb-3 size-11 rounded-xl object-contain"
              />
            ) : null}
            <h1 className="text-[26px] font-semibold tracking-tight">{heading}</h1>
            {description?.trim() ? (
              <p className="text-muted-foreground text-sm leading-relaxed">{description.trim()}</p>
            ) : null}
          </header>

          {/* 登录 / 注册页签（静态还原真实页面） */}
          <div className="mt-6 flex justify-start gap-6 border-b">
            <span
              className="pt-1 pb-2.5 text-sm font-medium"
              style={
                mode === 'signin'
                  ? { color: accent, borderBottom: `2px solid ${accent}` }
                  : { color: 'var(--muted-foreground)', borderBottom: '2px solid transparent' }
              }
            >
              {t('auth.signIn')}
            </span>
            {allowSignUp ? (
              <span
                className="pt-1 pb-2.5 text-sm font-medium"
                style={
                  mode === 'signup'
                    ? { color: accent, borderBottom: `2px solid ${accent}` }
                    : { color: 'var(--muted-foreground)', borderBottom: '2px solid transparent' }
                }
              >
                {t('auth.signUp')}
              </span>
            ) : null}
          </div>

          {mode === 'signup' ? (
            <div className="space-y-4 pt-6">
              <div className="space-y-2">
                <Label htmlFor="preview-signup-username">{t('auth.username')}</Label>
                <Input id="preview-signup-username" readOnly className="h-10" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="preview-signup-email">{t('auth.email')}</Label>
                <Input id="preview-signup-email" readOnly className="h-10" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="preview-signup-password">{t('auth.password')}</Label>
                <Input id="preview-signup-password" readOnly type="password" className="h-10" />
              </div>
              <Button type="button" className="h-10 w-full" style={{ backgroundColor: accent }}>
                {t('auth.signUp')}
              </Button>
            </div>
          ) : (
            <div className="space-y-4 pt-6">
              {hasPassword ? (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="preview-username">{t('auth.username')}</Label>
                    <Input
                      id="preview-username"
                      readOnly
                      placeholder={t('auth.usernamePlaceholder')}
                      className="h-10"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="preview-password">{t('auth.password')}</Label>
                    <Input id="preview-password" readOnly type="password" className="h-10" />
                  </div>
                </>
              ) : null}

              {!hasPassword && hasCode ? (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="preview-code-target">{t('auth.email')}</Label>
                    <Input id="preview-code-target" readOnly className="h-10" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="preview-code">{t('auth.code')}</Label>
                    <Input id="preview-code" readOnly className="h-10" />
                  </div>
                </>
              ) : null}

              {!hasPassword && !hasCode && hasMagic ? (
                <div className="space-y-2">
                  <Label htmlFor="preview-magic">{t('auth.email')}</Label>
                  <Input id="preview-magic" readOnly className="h-10" />
                </div>
              ) : null}

              {!hasPassword && !hasCode && !hasMagic && !hasProvider ? (
                <p className="text-muted-foreground text-center text-xs">
                  {t('applications.previewNoMethods')}
                </p>
              ) : null}

              <Button type="button" className="h-10 w-full" style={{ backgroundColor: accent }}>
                {t('auth.signIn')}
              </Button>

              {/* 其他可用入口：以图标示意（验证码 / 魔法链接 / 第三方） */}
              {/* 其他可用入口：图标与真实登录页共用同一套定义（SigninMethodIcon） */}
              {hasPassword && (hasCode || hasMagic || hasProvider) ? (
                <div className="text-muted-foreground flex items-center justify-center gap-3 pt-1">
                  {hasCode ? <SigninMethodIcon name="SMSCode" className="size-4" /> : null}
                  {hasMagic ? <SigninMethodIcon name="MagicLink" className="size-4" /> : null}
                  {hasProvider ? (
                    <span className="flex items-center gap-1 text-xs">
                      <SigninMethodIcon name="Provider" className="size-4" />
                      {t('auth.thirdParty')}
                    </span>
                  ) : null}
                </div>
              ) : null}

              {allowSignUp ? (
                <p className="text-muted-foreground text-center text-xs">
                  {t('applications.previewSignUpEntry')}
                </p>
              ) : null}
            </div>
          )}
        </div>
      </main>

      {/* 页脚 */}
      <footer className="text-muted-foreground px-6 pb-6 text-center text-xs">
        {t('auth.secureConnection')}
      </footer>
    </div>
  )
}
