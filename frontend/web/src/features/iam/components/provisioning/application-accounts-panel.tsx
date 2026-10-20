import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Plus } from 'lucide-react'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/shared/ui'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import { useAuth } from '@/shared/auth/auth-context'
import type { AppAccount } from '@/shared/api/types'
import { appAccountApi } from '../../api'
import { useListQuery } from '../../hooks/use-list-query'
import { useMutation } from '../../hooks/use-mutation'
import { Field } from '../field'
import { ResourceTable, type Column } from '../resource-table'
import { UserPicker } from '../user-picker'

/** 应用账号状态对应的徽标样式 */
function statusVariant(status: string): 'success' | 'destructive' | 'outline' {
  if (status === 'active') return 'success'
  if (status === 'failed') return 'destructive'
  return 'outline'
}

interface ApplicationAccountsPanelProps {
  institution: string
  applicationId: number
  applicationCode: string
}

/**
 * 应用详情内的应用账号台账。
 *
 * 应用由路由固定，用户只需要选择本地用户；建号 / 重试 / 删除都只作用于当前应用，
 * 不会再出现"在全局页面里手填应用标识"的误操作。
 */
export function ApplicationAccountsPanel({
  institution,
  applicationId,
  applicationCode,
}: ApplicationAccountsPanelProps) {
  const { t } = useTranslation()
  const { can } = useAuth()
  const canCreate = can('application_account', 'create')
  const canRetry = can('application_account', 'update')
  const canDelete = can('application_account', 'delete')

  const list = useListQuery<AppAccount>((params) => appAccountApi.list(params), {
    institution: institution || undefined,
    filters: { application_id: applicationId },
  })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [userIDs, setUserIDs] = useState<number[]>([])
  const [removing, setRemoving] = useState<AppAccount | null>(null)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!institution || userIDs.length === 0) return
    const ok = await run(
      () => appAccountApi.create({ institution, application_id: applicationId, user_id: userIDs[0] }),
      'appAccounts.createQueued',
    )
    if (ok) {
      setDialogOpen(false)
      setUserIDs([])
      await list.reload()
    }
  }

  const columns: Column<AppAccount>[] = [
    {
      key: 'user',
      headerKey: 'appAccounts.user',
      render: (account) => (
        <div className="flex max-w-[14rem] flex-col">
          <span className="truncate font-medium">{account.user}</span>
          <span className="text-muted-foreground truncate text-xs">{account.username}</span>
        </div>
      ),
    },
    {
      key: 'relatedUsername',
      headerKey: 'appAccounts.relatedUsername',
      render: (account) => <span className="font-mono text-xs">{account.related_username || '-'}</span>,
    },
    {
      key: 'createType',
      headerKey: 'appAccounts.createType',
      render: (account) => (
        <Badge variant="outline">
          {account.create_type === 'manual'
            ? t('appAccounts.createTypeManual')
            : t('appAccounts.createTypeAutomatic')}
        </Badge>
      ),
    },
    {
      key: 'status',
      headerKey: 'appAccounts.status',
      render: (account) => <Badge variant={statusVariant(account.status)}>{account.status}</Badge>,
    },
    ...(canRetry || canDelete
      ? [
          {
            key: 'actions',
            headerKey: 'common.actions',
            headClassName: 'text-right',
            render: (account: AppAccount) => (
              <RowActionsMenu
                primary={
                  canRetry
                    ? {
                        labelKey: 'appAccounts.retry',
                        disabled: pending,
                        onSelect: async () => {
                          const ok = await run(
                            () =>
                              appAccountApi.retry({
                                institution: institution || undefined,
                                application_id: applicationId,
                                user_id: account.user_id ?? 0,
                              }),
                            'appAccounts.retryQueued',
                          )
                          if (ok) await list.reload()
                        },
                      }
                    : undefined
                }
                actions={
                  canDelete
                    ? [{ labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(account) }]
                    : []
                }
              />
            ),
          },
        ]
      : []),
  ]

  return (
    <div className="space-y-4">
      <div className="border-border/60 bg-muted/30 rounded-xl border px-3.5 py-3">
        <p className="text-sm font-medium">{t('appAccounts.connectorTitle')}</p>
        <p className="text-muted-foreground mt-1 text-xs leading-relaxed">
          {t('appAccounts.connectorHint', { application: applicationCode })}{' '}
          <Button asChild variant="link" size="sm" className="inline h-auto p-0">
            <Link to={`/institutions/${institution}/connections?direction=out`}>
              {t('appAccounts.manageConnectors')}
            </Link>
          </Button>
        </p>
      </div>

      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="min-w-0 flex-1">
          <ListToolbar keyword={list.keyword} onKeywordChange={list.setKeyword} onSubmit={list.search} />
        </div>
        {canCreate ? (
          <Button onClick={() => setDialogOpen(true)}>
            <Plus className="size-4" /> {t('appAccounts.add')}
          </Button>
        ) : null}
      </div>

      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(account) => account.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('appAccounts.createTitle')}</DialogTitle>
            <DialogDescription>
              {t('appAccounts.createHint', { application: applicationCode })}
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <Field id="app-account-user" labelKey="appAccounts.user" hint={t('appAccounts.userHint')}>
              <UserPicker
                value={userIDs}
                onChange={(next) => setUserIDs(next.slice(-1))}
                institution={institution}
                placeholder={t('users.pickerPlaceholder')}
              />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={pending || userIDs.length === 0}>
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="appAccounts.deleteConfirmTitle"
        descriptionKey="appAccounts.deleteConfirmBody"
        descriptionParams={{ name: removing?.user ?? '', application: applicationCode }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(
            () => appAccountApi.remove(applicationId, removing.user_id ?? 0, institution || undefined),
            'appAccounts.deleteQueued',
          )
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </div>
  )
}
