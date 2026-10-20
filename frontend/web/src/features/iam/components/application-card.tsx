import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { AppWindow } from 'lucide-react'
import { Avatar, AvatarFallback, AvatarImage, Badge, Button } from '@/shared/ui'
import { CopyButton } from '@/shared/components/copy-button'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import { SigninMethodIcon } from '@/shared/components/signin-method-icon'
import { formatDateTime } from '@/shared/lib/format'
import type { Application } from '@/shared/api/types'
import { RESOURCE_CARD_CLASS } from './resource-cards'

interface ApplicationCardProps {
  app: Application
  /** 应用编辑向导地址（`/institutions/:inst/applications/:id`） */
  editHref: string
  onEndpoints: () => void
  onResetSecret: () => void
  onDelete: () => void
}

/** 单卡最多平铺的登录方式数量，超出部分折叠成「+N」 */
const METHOD_LIMIT = 3

/**
 * 应用卡片：品牌资源 + 名称/标识 + 状态，主体为登录方式与客户端 ID。
 *
 * 应用列表最常被问的三件事是"这是哪个应用 / 支持哪些登录方式 / client_id 是多少"，
 * 卡片按这个顺序自上而下排布；端点清单、重置密钥与删除属低频且危险，收进「更多」。
 */
export function ApplicationCard({
  app,
  editHref,
  onEndpoints,
  onResetSecret,
  onDelete,
}: ApplicationCardProps) {
  const { t } = useTranslation()
  const methods = app.signin_methods ?? []
  const hiddenCount = methods.length - METHOD_LIMIT

  return (
    <article className={RESOURCE_CARD_CLASS}>
      <header className="flex items-start gap-3">
        <Avatar className="size-10 rounded-[10px]">
          {app.logo ? <AvatarImage src={app.logo} alt="" className="object-cover" /> : null}
          <AvatarFallback className="bg-primary/10 text-primary rounded-[10px]">
            <AppWindow className="size-5" />
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <Link to={editHref} className="hover:text-primary block truncate text-sm font-semibold">
            {app.name || app.code}
          </Link>
          <div className="mt-1.5 flex flex-wrap items-center gap-1">
            <Badge variant="secondary">{app.type}</Badge>
            {/* 访问范围：受限时直接给出主体数，避免"以为限制了其实没有" */}
            <Badge variant={app.access_mode === 'restricted' ? 'outline' : 'secondary'}>
              {app.access_mode === 'restricted'
                ? t('applications.accessRestrictedBadge', { count: app.access_subject_count ?? 0 })
                : t('applications.accessAllBadge')}
            </Badge>
            <span className="text-muted-foreground truncate font-mono text-[11px]">{app.code}</span>
          </div>
        </div>
        <Badge variant={app.state === 'active' ? 'success' : 'destructive'}>
          {app.state === 'active' ? t('common.enabled') : t('common.disabled')}
        </Badge>
      </header>

      <p className="text-muted-foreground line-clamp-2 min-h-[34px] text-xs leading-relaxed">
        {app.description}
      </p>

      <div className="space-y-1.5">
        <p className="text-muted-foreground text-[11px]">{t('applications.enabledMethods')}</p>
        <div className="flex flex-wrap items-center gap-1">
          {methods.slice(0, METHOD_LIMIT).map((method) => (
            <Badge key={method.name} variant="outline">
              <SigninMethodIcon name={method.name} />
              {method.name}
            </Badge>
          ))}
          {hiddenCount > 0 ? (
            <Badge variant="secondary">{t('applications.moreMethods', { count: hiddenCount })}</Badge>
          ) : null}
          {methods.length === 0 ? (
            <span className="text-muted-foreground text-[11px]">{t('applications.noMethods')}</span>
          ) : null}
        </div>
      </div>

      <div className="border-border/70 bg-muted/40 flex items-center gap-2 rounded-xl border px-2.5 py-1.5">
        <span className="text-muted-foreground shrink-0 text-[11px]">{t('applications.clientId')}</span>
        <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{app.client_id}</code>
        <CopyButton value={app.client_id} label={t('applications.clientId')} />
      </div>

      <footer className="border-border/60 mt-auto flex items-center justify-between gap-2 border-t pt-3">
        <span className="text-muted-foreground truncate text-[11px]">
          {t('common.createdAt')} · {formatDateTime(app.created_at)}
        </span>
        <div className="flex shrink-0 items-center gap-1">
          <Button variant="outline" size="sm" asChild>
            <Link to={editHref}>{t('common.edit')}</Link>
          </Button>
          <RowActionsMenu
            actions={[
              { labelKey: 'applications.oauthEndpoints', onSelect: onEndpoints },
              { labelKey: 'applications.resetSecret', onSelect: onResetSecret },
              { labelKey: 'common.delete', onSelect: onDelete, destructive: true },
            ]}
          />
        </div>
      </footer>
    </article>
  )
}
