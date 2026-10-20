import { useTranslation } from 'react-i18next'
import { Building2, Check, ChevronsUpDown } from 'lucide-react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/shared/ui'
import { cn } from '@/shared/lib/utils'
import { useCurrentInstitution } from '@/features/iam/institution-context'

interface InstitutionSwitcherProps {
  className?: string
}

function initialOf(name: string) {
  return (name || '?').trim().slice(0, 1).toUpperCase()
}

/** 组织图标：有 logo 时直接展示，未配置时回退到名称首字母 */
function InstitutionAvatar({ logo, name, size }: { logo?: string; name: string; size: 'md' | 'sm' }) {
  const src = logo?.trim()
  const boxClass = size === 'md' ? 'size-8 rounded-[10px]' : 'size-7 rounded-md'

  if (src) {
    return <img src={src} alt="" className={cn('bg-muted/50 shrink-0 object-cover', boxClass)} />
  }

  return (
    <span
      className={cn(
        'bg-primary/10 text-primary grid shrink-0 place-items-center font-semibold',
        boxClass,
        size === 'md' ? 'text-[13px]' : 'text-[12px]',
      )}
    >
      {initialOf(name)}
    </span>
  )
}

/** 侧边栏顶部的组织切换器：切换后进入该组织的工作区并保留当前子路径 */
export function InstitutionSwitcher({ className }: InstitutionSwitcherProps) {
  const { t } = useTranslation()
  const { institutions, institutionId, setInstitution, loading } = useCurrentInstitution()
  const navigate = useNavigate()
  const { pathname } = useLocation()

  const current = institutions.find((item) => String(item.id) === institutionId)

  /** 切换组织：保留当前子路径（如 /users），地址栏组织段用主键 id */
  const switchTo = (id: number) => {
    setInstitution(String(id))
    const match = /^\/institutions\/[^/]+(\/.*)?$/.exec(pathname)
    navigate(`/institutions/${id}${match?.[1] ?? ''}`)
  }

  if (loading || institutions.length === 0) {
    return (
      <div className={cn('text-muted-foreground flex items-center gap-2 px-2 py-2 text-xs', className)}>
        <Building2 className="size-3.5" />
        {current?.display_name || institutionId || t('auth.institutionPlaceholder')}
      </div>
    )
  }

  return (
    <div className={cn('px-0.5', className)}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className="border-border/70 hover:bg-accent/60 data-[state=open]:bg-accent/60 flex w-full items-center gap-2.5 rounded-xl border px-2.5 py-2 text-left transition-colors"
            aria-label={t('institutions.switchCurrent', { name: current?.display_name || institutionId })}
          >
            <InstitutionAvatar logo={current?.logo} name={current?.display_name || institutionId} size="md" />
            <span className="grid min-w-0 flex-1 gap-0.5">
              <span className="text-muted-foreground text-[10px] leading-none">
                {t('institutions.currentHint')}
              </span>
              <span className="truncate text-[12px] leading-tight font-semibold">
                {current?.display_name || institutionId}
              </span>
            </span>
            <ChevronsUpDown className="text-muted-foreground size-3.5 shrink-0" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-[248px]">
          <DropdownMenuLabel className="text-muted-foreground text-[10px] font-semibold tracking-wide">
            {t('institutions.switch')}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          {institutions.map((option) => {
            const selected = String(option.id) === institutionId
            return (
              <DropdownMenuItem
                key={option.id}
                onSelect={() => switchTo(option.id)}
                className={cn('gap-2.5 py-1.5', selected && 'bg-primary/5')}
              >
                <InstitutionAvatar logo={option.logo} name={option.display_name || option.name} size="sm" />
                <span className="grid min-w-0 flex-1 gap-0.5">
                  <span className="truncate text-[12px] font-medium">
                    {option.display_name || option.name}
                  </span>
                  <span className="text-muted-foreground truncate text-[10px]">{option.name}</span>
                </span>
                {selected ? <Check className="text-primary size-4 shrink-0" /> : null}
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
