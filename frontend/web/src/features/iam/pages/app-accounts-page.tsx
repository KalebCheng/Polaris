import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
  Input,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import type { AppAccount } from '@/shared/api/types'
import { appAccountApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'
import { ResourceTable, type Column } from '../components/resource-table'

/** 应用账号状态对应的徽标样式 */
function statusVariant(status: string): 'success' | 'destructive' | 'outline' {
  if (status === 'active') return 'success'
  if (status === 'failed') return 'destructive'
  return 'outline'
}

const emptyForm = { application: '', user: '' }

/** 应用账号：已供给的下游账号列表，支持手动建号 / 重试 / 删除 */
export default function AppAccountsPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()

  const list = useListQuery<AppAccount>((params) => appAccountApi.list(params), {
    institution: institution || undefined,
  })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [removing, setRemoving] = useState<AppAccount | null>(null)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!institution) return
    const ok = await run(() =>
      appAccountApi.create({ institution, application: form.application, user: form.user }),
    )
    if (ok) {
      setDialogOpen(false)
      setForm(emptyForm)
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
      key: 'application',
      headerKey: 'appAccounts.application',
      render: (account) => <Badge variant="secondary">{account.application}</Badge>,
    },
    {
      key: 'relatedUsername',
      headerKey: 'appAccounts.relatedUsername',
      render: (account) => (
        <span className="font-mono text-xs">{account.related_username || '-'}</span>
      ),
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
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (account) => (
        <RowActionsMenu
          primary={{
            labelKey: 'appAccounts.retry',
            onSelect: async () => {
              const ok = await run(() =>
                appAccountApi.retry({
                  institution: institution || undefined,
                  application: account.application,
                  user: account.user,
                }),
              )
              if (ok) await list.reload()
            },
            disabled: pending,
          }}
          actions={[
            { labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(account) },
          ]}
        />
      ),
    },
  ]

  return (
    <PageShell
      titleKey="appAccounts.title"
      subtitleKey="appAccounts.subtitle"
      actions={
        <Button onClick={() => setDialogOpen(true)}>
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(account) => account.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        toolbar={
          <ListToolbar keyword={list.keyword} onKeywordChange={list.setKeyword} onSubmit={list.search} />
        }
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('appAccounts.createTitle')}</DialogTitle>
            <DialogDescription>{t('appAccounts.subtitle')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <Field id="app-account-application" labelKey="appAccounts.application">
              <Input
                id="app-account-application"
                value={form.application}
                onChange={(event) => setForm({ ...form, application: event.target.value })}
                placeholder="app-built-in"
                required
              />
            </Field>
            <Field id="app-account-user" labelKey="appAccounts.user">
              <Input
                id="app-account-user"
                value={form.user}
                onChange={(event) => setForm({ ...form, user: event.target.value })}
                required
              />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={pending}>
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: `${removing?.user ?? ''} @ ${removing?.application ?? ''}` }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(
            () => appAccountApi.remove(removing.user, removing.application, institution || undefined),
            'common.deleted',
          )
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
