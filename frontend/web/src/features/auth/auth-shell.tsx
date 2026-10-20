import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { Languages, Moon, Sun } from 'lucide-react'
import { BrandLockup } from '@/shared/components/brand-lockup'
import { Button } from '@/shared/ui'

/**
 * 认证页舞台（登录 / OAuth 回调共用）。
 *
 * 与控制台同一套设计语言：雾霭画布 + 玻璃面板 + 极光蓝主按钮，
 * 仅额外保留一层品牌光晕与极淡网格——认证页是唯一允许有氛围的页面。
 * 内容单列居中，所有视觉重量都收在一张玻璃卡上。
 */
export function AuthShell({
  children,
  brandName,
  brandLogo,
}: {
  children: ReactNode
  /** 页眉品牌名（默认平台名；组织品牌登录页传组织名） */
  brandName?: string
  /** 页眉品牌标识（默认内置矢量标识；组织品牌登录页传组织 Logo） */
  brandLogo?: string
}) {
  return (
    <div className="auth-scope relative min-h-dvh">
      <div className="relative z-10 flex min-h-dvh flex-col">
        <AuthMasthead brandName={brandName} brandLogo={brandLogo} />
        <main className="flex flex-1 items-center justify-center px-5 py-10 sm:px-8">
          {/* 单一悬浮层：整块登录内容装进一张玻璃卡，页面只保留这一层"实体" */}
          <div
            data-auth-content=""
            className="auth-rise surface-strong w-full max-w-[26rem] rounded-2xl p-7 sm:p-9"
          >
            {children}
          </div>
        </main>
        <AuthFootnote />
      </div>
    </div>
  )
}

/** 页眉：品牌标识 ＋ 语言 / 深浅色切换（尚未登录也能调整这两项偏好） */
function AuthMasthead({ brandName, brandLogo }: { brandName?: string; brandLogo?: string }) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme, setTheme } = useTheme()
  // next-themes 读取的是挂载后的实际主题，首帧先按深色渲染避免闪烁
  const [ready, setReady] = useState(false)
  useEffect(() => setReady(true), [])
  const current = i18n.resolvedLanguage ?? 'zh'
  const dark = !ready || resolvedTheme !== 'light'

  return (
    <header className="flex items-center justify-between gap-4 px-6 py-6 sm:px-8">
      <div className="flex items-center gap-2.5">
        <BrandLockup src={brandLogo} name={brandName} size="sm" animated />
      </div>
      <div className="flex items-center gap-0.5">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground hover:text-foreground size-9"
          aria-label={t('auth.themeSwitch')}
          title={t('auth.themeSwitch')}
          onClick={() => setTheme(dark ? 'light' : 'dark')}
        >
          {dark ? (
            <Moon className="size-4" aria-hidden="true" />
          ) : (
            <Sun className="size-4" aria-hidden="true" />
          )}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="text-muted-foreground hover:text-foreground h-9 gap-1.5 px-2.5"
          aria-label={t('auth.languageSwitch')}
          onClick={() => void i18n.changeLanguage(current.startsWith('zh') ? 'en' : 'zh')}
        >
          <Languages className="size-4" aria-hidden="true" />
          <span className="text-xs font-medium">{current.startsWith('zh') ? '中文' : 'EN'}</span>
        </Button>
      </div>
    </header>
  )
}

/** 页脚：一行克制的合规说明 */
function AuthFootnote() {
  const { t } = useTranslation()
  return (
    <footer className="text-muted-foreground px-6 pb-7 text-center text-xs sm:px-8">
      {t('auth.secureConnection')}
    </footer>
  )
}
