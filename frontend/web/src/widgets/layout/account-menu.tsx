import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import {
  Check,
  ChevronDown,
  LayoutDashboard,
  LayoutGrid,
  LogOut,
  Monitor,
  Moon,
  Sun,
  UserRound,
} from 'lucide-react'
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/shared/ui'
import { useTheme } from 'next-themes'
import { cn } from '@/shared/lib/utils'
import { useAuth } from '@/shared/auth/auth-context'

/** 顶栏账号菜单：语言 / 主题 / 个人资料 / 用户门户 / 退出登录 */
export function AccountMenu() {
  const { t, i18n } = useTranslation()
  const { status, signOut } = useAuth()
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)

  const displayName = status?.display_name || status?.name || ''
  const initial = (displayName || 'S').slice(0, 1).toUpperCase()
  const language = (i18n.resolvedLanguage ?? 'zh').startsWith('zh') ? 'zh' : 'en'

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={cn(
            'hover:bg-card/80 flex items-center gap-2 rounded-full py-1 pr-2.5 pl-1 transition-colors',
            open && 'bg-card/90',
          )}
        >
          <Avatar className="ring-card shadow-soft size-8 ring-2">
            {status?.avatar ? (
              <AvatarImage src={status.avatar} alt={displayName || t('auth.account')} />
            ) : null}
            <AvatarFallback className="bg-ink text-ink-foreground text-[11px] font-semibold">
              {initial}
            </AvatarFallback>
          </Avatar>
          <span className="hidden max-w-28 truncate text-[13px] font-medium sm:block">
            {displayName || '-'}
          </span>
          <ChevronDown className="text-muted-foreground hidden size-3.5 sm:block" />
        </button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuLabel className="font-normal">
          <span className="block text-sm font-medium">{displayName || '-'}</span>
          <span className="text-muted-foreground block truncate text-xs">
            {status?.institution_name ? `${status.institution_name} · ` : ''}
            {status?.email || status?.name}
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />

        <DropdownMenuSub>
          <DropdownMenuSubTrigger>{t('language.label')}</DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuItem onClick={() => void i18n.changeLanguage('zh')}>
              {language === 'zh' ? <Check className="size-4" /> : <span className="size-4" />}
              {t('language.zh')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => void i18n.changeLanguage('en')}>
              {language === 'en' ? <Check className="size-4" /> : <span className="size-4" />}
              {t('language.en')}
            </DropdownMenuItem>
          </DropdownMenuSubContent>
        </DropdownMenuSub>

        <DropdownMenuSub>
          <DropdownMenuSubTrigger>{t('theme.toggle')}</DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuItem onClick={() => setTheme('light')}>
              {theme === 'light' ? <Check className="size-4" /> : <Sun className="size-4" />}
              {t('theme.light')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setTheme('dark')}>
              {theme === 'dark' ? <Check className="size-4" /> : <Moon className="size-4" />}
              {t('theme.dark')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setTheme('system')}>
              {theme === 'system' ? <Check className="size-4" /> : <Monitor className="size-4" />}
              {t('theme.system')}
            </DropdownMenuItem>
          </DropdownMenuSubContent>
        </DropdownMenuSub>

        <DropdownMenuSeparator />
        {status?.is_admin ? (
          <DropdownMenuItem onClick={() => navigate('/')}>
            <LayoutDashboard className="size-4" /> {t('portal.backToConsole')}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem onClick={() => navigate('/portal')}>
          <LayoutGrid className="size-4" /> {t('portal.brandShort')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => navigate('/portal/profile')}>
          <UserRound className="size-4" /> {t('nav.profile')}
        </DropdownMenuItem>

        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          onClick={async () => {
            await signOut()
            navigate('/login', { replace: true })
          }}
        >
          <LogOut className="size-4" /> {t('auth.signOut')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
