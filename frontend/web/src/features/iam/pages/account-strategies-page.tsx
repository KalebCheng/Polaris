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
import { PageShell } from '@/widgets/layout/page-shell'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import type { AccountStrategy } from '@/shared/api/types'
import { accountStrategyApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'
import { ResourceTable, type Column } from '../components/resource-table'

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
  application: '',
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

/** 账号策略：建号 / 回收时机与本地→下游属性映射 */
export default function AccountStrategiesPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()

  const list = useListQuery<AccountStrategy>((params) => accountStrategyApi.list(params), {
    institution: institution || undefined,
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
      application: strategy.application ?? '',
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
      name: form.name,
      display_name: form.display_name,
      application: form.application,
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
      key: 'application',
      headerKey: 'accountStrategies.application',
      render: (strategy) =>
        strategy.application ? (
          <Badge variant="secondary">{strategy.application}</Badge>
        ) : (
          <span className="text-muted-foreground text-xs">-</span>
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
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (strategy) => (
        <RowActionsMenu
          primary={{ labelKey: 'common.edit', onSelect: () => openEdit(strategy), disabled: pending }}
          actions={[{ labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(strategy) }]}
        />
      ),
    },
  ]

  return (
    <PageShell
      titleKey="accountStrategies.title"
      subtitleKey="accountStrategies.subtitle"
      actions={
        <Button onClick={openCreate}>
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(strategy) => strategy.id}
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
              hint={t('accountStrategies.applicationHint')}
            >
              <Input
                id="strategy-application"
                value={form.application}
                onChange={(event) => setForm({ ...form, application: event.target.value })}
                placeholder="app-built-in"
              />
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
    </PageShell>
  )
}
