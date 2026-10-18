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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { StatusPill } from '@/shared/components/status-pill'
import type { Role } from '@/shared/api/types'
import { roleApi } from '../api'
import { useCurrentInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'
import { ResourceTable, type Column } from '../components/resource-table'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'

const emptyForm = { institution_id: 0, name: '', display_name: '', description: '' }

const splitList = (value: string) =>
  value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)

/** 角色管理（平台级，跨组织）：角色本身 + 成员/子角色/权限关联 */
export default function RolesPage() {
  const { t } = useTranslation()
  const { institutions, institution } = useCurrentInstitution()

  const list = useListQuery<Role>((params) => roleApi.list(params), { scope: 'platform' })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Role | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [removing, setRemoving] = useState<Role | null>(null)
  const [relationsOpen, setRelationsOpen] = useState(false)
  const [relationsTarget, setRelationsTarget] = useState<Role | null>(null)
  const [relations, setRelations] = useState({ users: '', jobTitles: '', roles: '', permissions: '' })

  const openCreate = () => {
    setEditing(null)
      setForm({ ...emptyForm, institution_id: Number(institution) || Number(institutions[0]?.id) || 0 })
    setDialogOpen(true)
  }

  const openEdit = (role: Role) => {
    setEditing(role)
    setForm({
      institution_id: Number(role.institution_id),
      name: role.name,
      display_name: role.display_name,
      description: role.description,
    })
    setDialogOpen(true)
  }

  const openRelations = async (role: Role) => {
    setRelationsTarget(role)
    setRelations({
      users: (role.users ?? []).join(', '),
      jobTitles: (role.job_titles ?? []).join(', '),
      roles: (role.roles ?? []).join(', '),
      permissions: '',
    })
    setRelationsOpen(true)
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const base = {
      institution_id: Number(form.institution_id),
      name: form.name,
      display_name: form.display_name,
      description: form.description,
      is_enabled: true,
    }
    const ok = editing
      ? await run(() => roleApi.update(editing.id, base, String(String(String(editing.institution_id)))))
      : await run(() => roleApi.create(base))
    if (ok) {
      setDialogOpen(false)
      await list.reload()
    }
  }

  const columns: Column<Role>[] = [
    {
      key: 'name',
      headerKey: 'roles.name',
      render: (role) => (
        <div className="flex items-center gap-2">
          <span className="text-foreground font-medium">{role.name}</span>
          {role.is_system ? <Badge variant="outline">{t('roles.isSystem')}</Badge> : null}
        </div>
      ),
    },
    {
      key: 'institution',
      headerKey: 'common.institution',
      render: (role) => <Badge variant="secondary">{role.institution_id}</Badge>,
    },
    { key: 'display', headerKey: 'roles.displayName', render: (role) => role.display_name || '-' },
    {
      key: 'members',
      headerKey: 'roles.users',
      className: 'text-xs',
      render: (role) => (role.users ?? []).join(', ') || '-',
    },
    {
      key: 'description',
      headerKey: 'roles.description',
      className: 'max-w-xs truncate text-xs',
      render: (role) => role.description || '-',
    },
    {
      key: 'state',
      headerKey: 'common.status',
      render: (role) => (
        <StatusPill tone={role.is_enabled ? 'success' : 'neutral'}>
          {role.is_enabled ? t('common.enabled') : t('common.disabled')}
        </StatusPill>
      ),
    },
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (role) => (
        <div className="flex justify-end">
          <RowActionsMenu
            primary={{ labelKey: 'common.edit', onSelect: () => openEdit(role) }}
            actions={[
              { labelKey: 'roles.relations', onSelect: () => void openRelations(role) },
              ...(role.is_system
                ? []
                : [{ labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(role) }]),
            ]}
          />
        </div>
      ),
    },
  ]

  return (
    <PageShell
      titleKey="roles.title"
      subtitleKey="roles.platformSubtitle"
      actions={
        <Button onClick={openCreate}>
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <SummaryStrip
        items={[
          { label: t('common.totalResources'), value: list.loading ? '—' : list.total },
          { label: t('common.manageScope'), value: t('common.scopePlatform') },
        ]}
      />
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(role) => role.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        unit={t('common.unitRole')}
        toolbar={
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
            placeholder={t('common.searchPlaceholder')}
            count={t('common.totalCount', { total: list.total, unit: t('common.unitRole') })}
          />
        }
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t('roles.editTitle') : t('roles.createTitle')}</DialogTitle>
            <DialogDescription>{t('roles.subtitle')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <Field id="role-institution" labelKey="common.institution">
              <Select
                  value={String(form.institution_id || '')}
                onValueChange={(value) => setForm({ ...form, institution_id: Number(value) })}
                disabled={Boolean(editing)}
              >
                <SelectTrigger id="role-institution">
                  <SelectValue placeholder={t('auth.institutionPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {institutions.map((option) => (
                    <SelectItem key={option.name} value={option.name}>
                      {option.display_name || option.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field id="role-name" labelKey="roles.name">
              <Input
                id="role-name"
                value={form.name}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
                disabled={Boolean(editing)}
                required
              />
            </Field>
            <Field id="role-display" labelKey="roles.displayName">
              <Input
                id="role-display"
                value={form.display_name}
                onChange={(event) => setForm({ ...form, display_name: event.target.value })}
              />
            </Field>
            <Field id="role-description" labelKey="roles.description">
              <Textarea
                id="role-description"
                value={form.description}
                onChange={(event) => setForm({ ...form, description: event.target.value })}
                rows={2}
              />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={pending || !form.institution_id}>
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={relationsOpen} onOpenChange={setRelationsOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('roles.relations')}</DialogTitle>
            <DialogDescription>{relationsTarget?.name}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <Field id="role-users" labelKey="roles.users" hint={t('permissions.commaSeparated')}>
              <Textarea
                id="role-users"
                value={relations.users}
                onChange={(event) => setRelations({ ...relations, users: event.target.value })}
                rows={2}
              />
            </Field>
            <Field id="role-job-titles" labelKey="roles.jobTitles" hint={t('permissions.commaSeparated')}>
              <Textarea
                id="role-job-titles"
                value={relations.jobTitles}
                onChange={(event) => setRelations({ ...relations, jobTitles: event.target.value })}
                rows={2}
              />
            </Field>
            <Field id="role-sub-roles" labelKey="roles.subRoles" hint={t('permissions.commaSeparated')}>
              <Textarea
                id="role-sub-roles"
                value={relations.roles}
                onChange={(event) => setRelations({ ...relations, roles: event.target.value })}
                rows={2}
              />
            </Field>
            <Field id="role-permissions" labelKey="roles.permissions" hint={t('permissions.commaSeparated')}>
              <Textarea
                id="role-permissions"
                value={relations.permissions}
                onChange={(event) => setRelations({ ...relations, permissions: event.target.value })}
                rows={2}
              />
            </Field>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRelationsOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              disabled={pending || !relationsTarget}
              onClick={async () => {
                if (!relationsTarget) return
                const ok = await run(() =>
                  roleApi.setRelations(
                    relationsTarget.id,
                    {
                      users: splitList(relations.users),
                      job_titles: splitList(relations.jobTitles),
                      roles: splitList(relations.roles),
                      permissions: splitList(relations.permissions),
                    },
                      String(relationsTarget.institution_id),
                  ),
                )
                if (ok) {
                  setRelationsOpen(false)
                  await list.reload()
                }
              }}
            >
              {t('common.save')}
            </Button>
          </DialogFooter>
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
            const ok = await run(() => roleApi.remove(removing.id, String(removing.institution_id)), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
