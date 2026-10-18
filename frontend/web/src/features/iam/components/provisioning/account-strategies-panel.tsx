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
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  Textarea,
} from '@/shared/ui'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import { useAuth } from '@/features/auth/auth-context'
import type { AccountStrategy } from '@/shared/api/types'
import { accountStrategyApi } from '../../api'
import { useListQuery } from '../../hooks/use-list-query'
import { useMutation } from '../../hooks/use-mutation'
import { Field } from '../field'
import { ResourceTable, type Column } from '../resource-table'

const createWhenOptions = ['manual', 'onCreate', 'onGrant', 'jit'] as const
const revokeWhenOptions = ['disable', 'delete', 'offboard'] as const

const createWhenLabelKeys: Record<string, string> = {
  manual: 'accountStrategies.createWhenManual',
  onCreate: 'accountStrategies.createWhenOnCreate',
  onGrant: 'accountStrategies.createWhenOnGrant',
  jit: 'accountStrategies.createWhenJit',
}

const revokeWhenLabelKeys: Record<string, string> = {
  disable: 'accountStrategies.revokeWhenDisable',
  delete: 'accountStrategies.revokeWhenDelete',
  offboard: 'accountStrategies.revokeWhenOffboard',
}

const emptyForm = {
  name: '',
  display_name: '',
  is_enabled: true,
  createWhen: 'onCreate',
  revokeWhen: 'disable',
  mapping: '',
}

/** 把 Record<string,string> 展开为 "local=downstream" 的多行文本 */
function mappingToText(mapping: Record<string, string>): string {
  return Object.entries(mapping ?? {})
    .map(([local, downstream]) => `${local}=${downstream}`)
    .join('\n')
}

/** 把多行 "local=downstream" 文本解析为映射对象 */
function textToMapping(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const index = trimmed.indexOf('=')
    if (index <= 0) continue
    const local = trimmed.slice(0, index).trim()
    const downstream = trimmed.slice(index + 1).trim()
    if (local && downstream) out[local] = downstream
  }
  return out
}

interface AccountStrategiesPanelProps {
  institution: string
  applicationId: number
  applicationCode: string
}

/**
 * 应用详情内的账号策略面板。
 *
 * 策略按应用维护（application_id），因此这里不再让用户手填应用标识；
 * 新建 / 编辑一律携带当前应用主键，避免出现"填了 code、运行时却按默认策略走"的假配置。
 */
export function AccountStrategiesPanel({
  institution,
  applicationId,
  applicationCode,
}: AccountStrategiesPanelProps) {
  const { t } = useTranslation()
  const { can } = useAuth()
  const canCreate = can('account_strategy', 'create')
  const canUpdate = can('account_strategy', 'update')
  const canDelete = can('account_strategy', 'delete')

  const list = useListQuery<AccountStrategy>((params) => accountStrategyApi.list(params), {
    institution: institution || undefined,
    filters: { application_id: applicationId },
  })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<AccountStrategy | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [removing, setRemoving] = useState<AccountStrategy | null>(null)

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setDialogOpen(true)
  }

  const openEdit = (strategy: AccountStrategy) => {
    setEditing(strategy)
    setForm({
      name: strategy.name,
      display_name: strategy.display_name,
      is_enabled: strategy.is_enabled,
      createWhen: strategy.policy?.createWhen ?? 'onCreate',
      revokeWhen: strategy.policy?.revokeWhen ?? 'disable',
      mapping: mappingToText(strategy.mapping),
    })
    setDialogOpen(true)
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!institution) return
    const payload: Partial<AccountStrategy> = {
      institution_id: Number(institution),
      application_id: applicationId,
      application: applicationCode,
      name: form.name,
      display_name: form.display_name,
      is_enabled: form.is_enabled,
      mapping: textToMapping(form.mapping),
      policy: { createWhen: form.createWhen, revokeWhen: form.revokeWhen },
    }
    const ok = editing
      ? await run(() => accountStrategyApi.update(editing.id, payload))
      : await run(() => accountStrategyApi.create(payload))
    if (ok) {
      setDialogOpen(false)
      await list.reload()
    }
  }

  const columns: Column<AccountStrategy>[] = [
    {
      key: 'name',
      headerKey: 'accountStrategies.name',
      render: (strategy) => (
        <div className="flex max-w-[14rem] flex-col">
          <span className="truncate font-medium">{strategy.name}</span>
          <span className="text-muted-foreground truncate text-xs">{strategy.display_name}</span>
        </div>
      ),
    },
    {
      key: 'createWhen',
      headerKey: 'accountStrategies.createWhen',
      render: (strategy) => (
        <Badge variant="outline">
          {t(createWhenLabelKeys[strategy.policy?.createWhen ?? 'onCreate'] ?? 'accountStrategies.createWhenOnCreate')}
        </Badge>
      ),
    },
    {
      key: 'revokeWhen',
      headerKey: 'accountStrategies.revokeWhen',
      render: (strategy) => (
        <Badge variant="outline">
          {t(revokeWhenLabelKeys[strategy.policy?.revokeWhen ?? 'disable'] ?? 'accountStrategies.revokeWhenDisable')}
        </Badge>
      ),
    },
    {
      key: 'enabled',
      headerKey: 'accountStrategies.enabled',
      render: (strategy) => (
        <Badge variant={strategy.is_enabled ? 'success' : 'outline'}>
          {strategy.is_enabled ? t('common.enabled') : t('common.disabled')}
        </Badge>
      ),
    },
    ...(canUpdate || canDelete
      ? [{
          key: 'actions',
          headerKey: 'common.actions',
          headClassName: 'text-right',
          render: (strategy: AccountStrategy) => (
            <RowActionsMenu
              primary={
                canUpdate
                  ? { labelKey: 'common.edit', onSelect: () => openEdit(strategy), disabled: pending }
                  : undefined
              }
              actions={
                canDelete
                  ? [{ labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(strategy) }]
                  : []
              }
            />
          ),
        }]
      : []),
  ]

  return (
    <div className="space-y-4">
      <div className="border-border/60 bg-muted/30 rounded-xl border px-3.5 py-3">
        <p className="text-sm font-medium">{t('accountStrategies.applicationScopedTitle')}</p>
        <p className="text-muted-foreground mt-1 text-xs leading-relaxed">
          {t('accountStrategies.applicationScopedHint', { application: applicationCode })}
        </p>
      </div>

      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="min-w-0 flex-1">
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
          />
        </div>
        {canCreate ? (
          <Button onClick={openCreate}>
            <Plus className="size-4" /> {t('accountStrategies.add')}
          </Button>
        ) : null}
      </div>

      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(strategy) => strategy.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing ? t('accountStrategies.editTitle') : t('accountStrategies.createTitle')}
            </DialogTitle>
            <DialogDescription>{t('accountStrategies.subtitle')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="strategy-name" labelKey="accountStrategies.name">
                <Input
                  id="strategy-name"
                  value={form.name}
                  onChange={(event) => setForm({ ...form, name: event.target.value })}
                  disabled={Boolean(editing)}
                  required
                />
              </Field>
              <Field id="strategy-display" labelKey="accountStrategies.displayName">
                <Input
                  id="strategy-display"
                  value={form.display_name}
                  onChange={(event) => setForm({ ...form, display_name: event.target.value })}
                />
              </Field>
            </div>

            <Field
              id="strategy-application"
              labelKey="accountStrategies.application"
              hint={t('accountStrategies.applicationLockedHint')}
            >
              <div
                id="strategy-application"
                className="border-input/70 bg-muted/40 flex h-10 items-center rounded-xl border px-3.5 text-[13px]"
              >
                <Badge variant="secondary">{applicationCode}</Badge>
              </div>
            </Field>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="strategy-create" labelKey="accountStrategies.createWhen">
                <Select
                  value={form.createWhen}
                  onValueChange={(value) => setForm({ ...form, createWhen: value })}
                >
                  <SelectTrigger id="strategy-create">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {createWhenOptions.map((option) => (
                      <SelectItem key={option} value={option}>
                        {t(createWhenLabelKeys[option])}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field id="strategy-revoke" labelKey="accountStrategies.revokeWhen">
                <Select
                  value={form.revokeWhen}
                  onValueChange={(value) => setForm({ ...form, revokeWhen: value })}
                >
                  <SelectTrigger id="strategy-revoke">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {revokeWhenOptions.map((option) => (
                      <SelectItem key={option} value={option}>
                        {t(revokeWhenLabelKeys[option])}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>

            <Field id="strategy-mapping" labelKey="accountStrategies.mapping" hint={t('accountStrategies.mappingHint')}>
              <Textarea
                id="strategy-mapping"
                rows={4}
                value={form.mapping}
                onChange={(event) => setForm({ ...form, mapping: event.target.value })}
                placeholder={'email=mail\nphone=mobile'}
              />
            </Field>

            <div className="flex items-center gap-2">
              <Switch
                id="strategy-enabled"
                checked={form.is_enabled}
                onCheckedChange={(checked) => setForm({ ...form, is_enabled: checked })}
              />
              <Label htmlFor="strategy-enabled">{t('accountStrategies.enabled')}</Label>
            </div>

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
        descriptionParams={{ name: removing?.name }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(() => accountStrategyApi.remove(removing.id), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </div>
  )
}
