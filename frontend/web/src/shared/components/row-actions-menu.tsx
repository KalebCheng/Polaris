import { useTranslation } from 'react-i18next'
import { MoreHorizontal } from 'lucide-react'
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/shared/ui'

export interface RowAction {
  labelKey: string
  onSelect: () => void
  /** 危险操作（红色呈现，自动置于分隔线之后） */
  destructive?: boolean
  disabled?: boolean
}

interface RowActionsMenuProps {
  /** 常驻展示的主操作（最多 1 个，避免挤压关键列） */
  primary?: RowAction
  /** 收进「更多」菜单的操作 */
  actions?: RowAction[]
}

/**
 * 行操作菜单：主操作常驻 + 其余收进下拉。
 * 目的：避免一行堆多个文字按钮把名称/标识等关键列挤到换行。
 */
export function RowActionsMenu({ primary, actions = [] }: RowActionsMenuProps) {
  const { t } = useTranslation()
  const normal = actions.filter((action) => !action.destructive)
  const destructive = actions.filter((action) => action.destructive)

  return (
    <div className="flex items-center justify-end gap-1">
      {primary ? (
        <Button variant="outline" size="sm" disabled={primary.disabled} onClick={primary.onSelect}>
          {t(primary.labelKey)}
        </Button>
      ) : null}
      {actions.length > 0 ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label={t('common.actions')}>
              <MoreHorizontal className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            {normal.map((action) => (
              <DropdownMenuItem key={action.labelKey} disabled={action.disabled} onClick={action.onSelect}>
                {t(action.labelKey)}
              </DropdownMenuItem>
            ))}
            {normal.length > 0 && destructive.length > 0 ? <DropdownMenuSeparator /> : null}
            {destructive.map((action) => (
              <DropdownMenuItem
                key={action.labelKey}
                variant="destructive"
                disabled={action.disabled}
                onClick={action.onSelect}
              >
                {t(action.labelKey)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  )
}
