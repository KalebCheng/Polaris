import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ArrowRight, Globe } from 'lucide-react'
import { Avatar, AvatarFallback, AvatarImage, Badge, Button } from '@/shared/ui'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import { StatusPill } from '@/shared/components/status-pill'
import { formatDateTime } from '@/shared/lib/format'
import { isBuiltInInstitution } from '@/shared/lib/builtin'
import type { Institution } from '@/shared/api/types'
import { RESOURCE_CARD_CLASS } from './resource-cards'

interface InstitutionCardProps {
  org: Institution
  onEdit: () => void
  onDelete: () => void
}

/**
 * 组织卡片：图标 + 名称/标识 + 运行状态，底部为进入工作区与次级操作。
 *
 * 「进入工作区」是组织列表的主操作，因此常驻（表格里它是名称上的小链接，很容易错过）；
 * 编辑与删除收进「更多」，避免在卡片里排一排文字按钮把标识挤到换行。
 */
export function InstitutionCard({ org, onEdit, onDelete }: InstitutionCardProps) {
  const { t } = useTranslation()
  const builtIn = isBuiltInInstitution(org.name)
  const name = org.display_name || org.name

  const actions = [
    { labelKey: 'common.edit', onSelect: onEdit },
    // 内置组织属于平台自身，后端同样拒绝删除——这里直接不给入口，而不是点完再报错
    ...(builtIn ? [] : [{ labelKey: 'common.delete', onSelect: onDelete, destructive: true }]),
  ]

  return (
    <article className={RESOURCE_CARD_CLASS}>
      <header className="flex items-start gap-3">
        <Avatar className="size-10 rounded-[10px]">
          {org.logo ? <AvatarImage src={org.logo} alt="" className="object-cover" /> : null}
          <AvatarFallback className="bg-primary/10 text-primary rounded-[10px] text-sm font-semibold">
            {name.slice(0, 1).toUpperCase()}
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <Link
            to={`/institutions/${org.id}`}
            className="hover:text-primary block truncate text-sm font-semibold"
          >
            {name}
          </Link>
          <div className="mt-1.5 flex flex-wrap items-center gap-1">
            <Badge variant="secondary" className="font-mono">
              {org.name}
            </Badge>
            {builtIn ? <Badge variant="outline">{t('institutions.builtIn')}</Badge> : null}
          </div>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1.5">
          <StatusPill tone="success">{t('institutions.running')}</StatusPill>
          {org.disable_signin ? (
            <StatusPill tone="warning">{t('institutions.disableSignin')}</StatusPill>
          ) : null}
        </div>
      </header>

      <p className="text-muted-foreground line-clamp-2 min-h-[34px] text-xs leading-relaxed">
        {org.description}
      </p>

      {org.website_url ? (
        <p className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
          <Globe className="size-3.5 shrink-0" />
          <span className="truncate">{org.website_url}</span>
        </p>
      ) : null}

      <footer className="border-border/60 mt-auto flex items-center justify-between gap-2 border-t pt-3">
        <span className="text-muted-foreground truncate text-[11px]">
          {t('common.createdAt')} · {formatDateTime(org.created_at)}
        </span>
        <div className="flex shrink-0 items-center gap-1">
          <Button variant="outline" size="sm" asChild>
            <Link to={`/institutions/${org.id}`}>
              {t('institutions.enterWorkspace')}
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
          <RowActionsMenu actions={actions} />
        </div>
      </footer>
    </article>
  )
}
