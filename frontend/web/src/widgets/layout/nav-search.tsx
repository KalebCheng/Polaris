import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { CornerDownLeft, Search } from 'lucide-react'
import { Dialog, DialogContent, DialogTitle, Input } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'
import { useAuth } from '@/features/auth/auth-context'
import { useCurrentInstitution } from '@/features/iam/institution-context'
import { platformNav, securityTabs, settingsTabs, workspacePath, workspaceSections } from './nav-config'

interface NavTarget {
  label: string
  to: string
  group: 'platform' | 'workspace'
}

/**
 * 全局快速跳转（顶栏搜索）。
 *
 * 存在的理由：控制台有两层导航（平台级 + 当前组织），资源多时"我想要的页面在哪个菜单"
 * 比"我怎么用这个页面"更费时间。这里把两级导航**拍平成一个可搜索的跳转面板**，
 * 直接复用 `nav-config` 的导航结构与权限门禁——不新增一份菜单清单，因此不会漂移。
 *
 * 菜单里看得见的，这里搜得到；菜单里因权限隐藏的，这里也不出现（同一套 `can()` 判定）。
 */
export function NavSearch({ className }: { className?: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { can } = useAuth()
  const { institutionId } = useCurrentInstitution()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  const targets = useMemo<NavTarget[]>(() => {
    // 「设置」在菜单里是一个入口，在搜索里则直达它的二级页（存储 / 任务中心 / 审计日志）：
    // 父级入口不单独列出，否则会出现两条同址结果（同一 `to` 还会撞 React key）。
    const platform: NavTarget[] = platformNav
      .filter((item) => !item.to.startsWith('/settings'))
      .map((item) => ({
        label: t(item.labelKey),
        to: item.to,
        group: 'platform',
      }))
    const security: NavTarget[] = securityTabs.map((tab) => ({
      label: t(tab.labelKey),
      to: tab.to,
      group: 'platform',
    }))
    const settings: NavTarget[] = settingsTabs.map((tab) => ({
      label: t(tab.labelKey),
      to: tab.to,
      group: 'platform',
    }))
    // 组织内资源需要组织上下文；没有当前组织时只列平台级目标
    const workspace: NavTarget[] = institutionId
      ? workspaceSections.flatMap((section) =>
          section.items
            .filter((item) => !item.resource || can(item.resource, 'read'))
            .map((item) => ({
              label: t(item.labelKey),
              to: workspacePath(institutionId, item.to),
              group: 'workspace' as const,
            })),
        )
      : []
    return [...platform, ...security, ...settings, ...workspace]
  }, [t, can, institutionId])

  const filtered = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return targets
    return targets.filter((target) => target.label.toLowerCase().includes(keyword))
  }, [query, targets])

  // 结果集变化后把高亮拉回第一项，避免"搜窄了但高亮停在已消失的第 5 项"
  useEffect(() => {
    setActiveIndex(0)
  }, [query, open])

  // 键盘上下移动时把高亮项滚进视野（结果多于面板高度时）
  useEffect(() => {
    const active = listRef.current?.querySelector('[data-active="true"]')
    active?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex, filtered.length])

  const go = (target: NavTarget) => {
    setOpen(false)
    setQuery('')
    navigate(target.to)
  }

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((index) => (filtered.length === 0 ? 0 : (index + 1) % filtered.length))
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((index) => (filtered.length === 0 ? 0 : (index - 1 + filtered.length) % filtered.length))
      return
    }
    if (event.key === 'Enter') {
      const target = filtered[activeIndex]
      if (target) {
        event.preventDefault()
        go(target)
      }
    }
  }

  const groupLabel = (group: NavTarget['group']) =>
    group === 'platform' ? t('nav.searchGroupPlatform') : t('nav.searchGroupWorkspace')

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={cn(
          'border-border/50 bg-card/70 text-muted-foreground hover:bg-card hover:text-foreground',
          'inline-flex h-9 items-center gap-2 rounded-full border px-3.5 text-[13px] transition-colors',
          className,
        )}
        aria-label={t('nav.search')}
      >
        <Search className="size-3.5 shrink-0" />
        <span className="truncate">{t('nav.searchHint')}</span>
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          showCloseButton={false}
          className="top-[8%] translate-y-0 gap-0 rounded-2xl p-0 shadow-pop sm:max-w-xl"
          innerClassName="gap-0 p-0"
          aria-describedby={undefined}
        >
          <DialogTitle className="sr-only">{t('nav.search')}</DialogTitle>
          <div className="flex items-center gap-3 border-b px-4">
            <Search className="text-muted-foreground size-4 shrink-0" />
            {/* eslint-disable-next-line jsx-a11y/no-autofocus -- 面板的唯一目的是输入，打开即聚焦 */}
            <Input
              autoFocus
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={onKeyDown}
              placeholder={t('nav.searchHint')}
              aria-label={t('nav.search')}
              className="h-12 border-0 bg-transparent px-0 text-[14px] shadow-none focus-visible:ring-0"
            />
            <kbd className="border-border text-muted-foreground hidden shrink-0 rounded-md border px-1.5 py-0.5 text-[10px] sm:block">
              ESC
            </kbd>
          </div>

          <div ref={listRef} role="listbox" aria-label={t('nav.search')} className="max-h-[22rem] overflow-y-auto p-2">
            {filtered.length === 0 ? (
              <p className="text-muted-foreground px-3 py-8 text-center text-[13px]">{t('nav.searchEmpty')}</p>
            ) : (
              filtered.map((target, index) => {
                const previous = filtered[index - 1]
                const showGroup = !previous || previous.group !== target.group
                const active = index === activeIndex
                return (
                  <div key={target.to}>
                    {showGroup ? (
                      <p className="text-muted-foreground px-3 pt-3 pb-1 text-[10px] font-semibold tracking-wide">
                        {groupLabel(target.group)}
                      </p>
                    ) : null}
                    <button
                      type="button"
                      role="option"
                      aria-selected={active}
                      data-active={active}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={() => go(target)}
                      className={cn(
                        'flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-[13px] transition-colors',
                        active ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-accent/60',
                      )}
                    >
                      <span className="truncate">{target.label}</span>
                      {active ? <CornerDownLeft className="size-3.5 shrink-0 opacity-60" /> : null}
                    </button>
                  </div>
                )
              })
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
