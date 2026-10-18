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
  Textarea,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import type { Permission } from '@/shared/api/types'
import { permissionApi } from '../api'
import { useCurrentInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'
import { ResourceTable, type Column } from '../components/resource-table'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'

const emptyForm = {
  institution_id: 0,
  name: '',
  display_name: '',
  description: '',
  resources: 'user',
  actions: 'read',
  effect: 'Allow',
}

const splitList = (value: string) =>
  value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)

/** 权限管理（平台级，跨组织）：资源-动作授权 + 主体绑定 + 鉴权测试 */
export default function PermissionsPage() {
  const { t } = useTranslation()
  const { institutions, institution } = useCurrentInstitution()

  const list = useListQuery<Permission>((params) => permissionApi.list(params), { scope: 'platform' })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Permission | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [removing, setRemoving] = useState<Permission | null>(null)

  const [subjectsOpen, setSubjectsOpen] = useState(false)
  const [subjectsTarget, setSubjectsTarget] = useState<Permission | null>(null)
  const [subjects, setSubjects] = useState({ users: '', jobTitles: '', roles: '' })

  const [enforceOpen, setEnforceOpen] = useState(false)
  const [enforceForm, setEnforceForm] = useState({
    institution_id: 0,
    user: '',
    resource: 'user',
    action: 'read',
  })
  const [enforceResult, setEnforceResult] = useState<boolean | null>(null)

  const openCreate = () => {
    setEditing(null)
    setForm({ ...emptyForm, institution_id: Number(institution) || Number(institutions[0]?.id) || 0 })
    setDialogOpen(true)
  }

  const openEdit = (permission: Permission) => {
    setEditing(permission)
    setForm({
      institution_id: Number(permission.institution_id),
      name: permission.name,
      display_name: permission.display_name,
      description: permission.description,
      resources: permission.resources.join(', '),
      actions: permission.actions.join(', '),
      effect: permission.effect || 'Allow',
    })
    setDialogOpen(true)
  }

  const openSubjects = (permission: Permission) => {
    setSubjectsTarget(permission)
    setSubjects({
      users: (permission.users ?? []).join(', '),
      jobTitles: (permission.job_titles ?? []).join(', '),
      roles: (permission.roles ?? []).join(', '),
    })
    setSubjectsOpen(true)
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const payload = {
      institution_id: Number(form.institution_id),
      name: form.name,
      display_name: form.display_name,
      description: form.description,
      resources: splitList(form.resources),
      actions: splitList(form.actions),
      effect: form.effect,
      is_enabled: true,
      state: 'Approved',
    }
    const ok = editing
      ? await run(() => permissionApi.update(editing.id, payload, String(editing.institution_id)))
      : await run(() => permissionApi.create(payload))
    if (ok) {
      setDialogOpen(false)
      await list.reload()
    }
  }

  const columns: Column<Permission>[] = [
    {
      key: 'name',
      headerKey: 'permissions.name',
      render: (permission) => (
        <div className="flex flex-col">
          <span className="text-foreground font-medium">{permission.name}</span>
          <span className="text-muted-foreground text-xs">{permission.display_name}</span>
        </div>
      ),
    },
    {
      key: 'institution',
      headerKey: 'common.institution',
      render: (permission) => <Badge variant="secondary">{permission.institution_id}</Badge>,
    },
    {
      key: 'resources',
      headerKey: 'permissions.resources',
      render: (permission) => (
        <div className="flex flex-wrap gap-1">
          {permission.resources.map((resource) => (
            <Badge key={resource} variant="secondary">
              {resource}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      key: 'actions',
      headerKey: 'permissions.actions',
      render: (permission) => (
        <div className="flex flex-wrap gap-1">
          {permission.actions.map((action) => (
            <Badge key={action} variant="outline">
              {action}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      key: 'effect',
      headerKey: 'permissions.effect',
      render: (permission) => (
        <Badge variant={permission.effect === 'Deny' ? 'destructive' : 'success'}>
          {permission.effect === 'Deny' ? t('permissions.deny') : t('permissions.allow')}
        </Badge>
      ),
    },
    {
      key: 'actions-col',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (permission) => (
        <div className="flex justify-end">
          <RowActionsMenu
            primary={{ labelKey: 'common.edit', onSelect: () => openEdit(permission) }}
            actions={[
              { labelKey: 'permissions.subjects', onSelect: () => openSubjects(permission) },
              { labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(permission) },
            ]}
          />
        </div>
      ),
    },
  ]

  return (
    <PageShell
      titleKey="permissions.title"
      subtitleKey="permissions.platformSubtitle"
      actions={
        <>
          <Button
            variant="outline"
            onClick={() => {
              setEnforceResult(null)
              setEnforceForm({
                institution_id: Number(institution) || Number(institutions[0]?.id) || 0,
                user: '',
                resource: 'user',
                action: 'read',
              })
              setEnforceOpen(true)
            }}
          >
            {t('permissions.enforce')}
          </Button>
          <Button onClick={openCreate}>
            <Plus className="size-4" /> {t('common.create')}
          </Button>
        </>
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
        rowKey={(permission) => permission.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        unit={t('common.unitPermission')}
        toolbar={
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
            count={t('common.totalCount', { total: list.total, unit: t('common.unitPermission') })}
          />
        }
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t('permissions.editTitle') : t('permissions.createTitle')}</DialogTitle>
            <DialogDescription>{t('permissions.subtitle')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <Field id="permission-institution" labelKey="common.institution">
              <Select
                value={String(form.institution_id || '')}
                onValueChange={(value) => setForm({ ...form, institution_id: Number(value) })}
                disabled={Boolean(editing)}
              >
                <SelectTrigger id="permission-institution">
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
            <Field id="permission-name" labelKey="permissions.name">
              <Input
                id="permission-name"
                value={form.name}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
                disabled={Boolean(editing)}
                required
              />
            </Field>
            <Field id="permission-display" labelKey="permissions.displayName">
              <Input
                id="permission-display"
                value={form.display_name}
                onChange={(event) => setForm({ ...form, display_name: event.target.value })}
              />
            </Field>
            <Field
              id="permission-resources"
              labelKey="permissions.resources"
              hint={t('permissions.commaSeparated')}
            >
              <Input
                id="permission-resources"
                value={form.resources}
                onChange={(event) => setForm({ ...form, resources: event.target.value })}
              />
            </Field>
            <Field
              id="permission-actions"
              labelKey="permissions.actions"
              hint={t('permissions.commaSeparated')}
            >
              <Input
                id="permission-actions"
                value={form.actions}
                onChange={(event) => setForm({ ...form, actions: event.target.value })}
              />
            </Field>
            <Field id="permission-effect" labelKey="permissions.effect">
              <Select value={form.effect} onValueChange={(value) => setForm({ ...form, effect: value })}>
                <SelectTrigger id="permission-effect">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="Allow">{t('permissions.allow')}</SelectItem>
                  <SelectItem value="Deny">{t('permissions.deny')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field id="permission-description" labelKey="roles.description">
              <Textarea
                id="permission-description"
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

      <Dialog open={subjectsOpen} onOpenChange={setSubjectsOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('permissions.subjects')}</DialogTitle>
            <DialogDescription>{subjectsTarget?.name}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <Field id="subject-users" labelKey="roles.users" hint={t('permissions.commaSeparated')}>
              <Textarea
                id="subject-users"
                value={subjects.users}
                onChange={(event) => setSubjects({ ...subjects, users: event.target.value })}
                rows={2}
              />
            </Field>
            <Field id="subject-job-titles" labelKey="roles.jobTitles" hint={t('permissions.commaSeparated')}>
              <Textarea
                id="subject-job-titles"
                value={subjects.jobTitles}
                onChange={(event) => setSubjects({ ...subjects, jobTitles: event.target.value })}
                rows={2}
              />
            </Field>
            <Field id="subject-roles" labelKey="roles.subRoles" hint={t('permissions.commaSeparated')}>
              <Textarea
                id="subject-roles"
                value={subjects.roles}
                onChange={(event) => setSubjects({ ...subjects, roles: event.target.value })}
                rows={2}
              />
            </Field>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSubjectsOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              disabled={pending || !subjectsTarget}
              onClick={async () => {
                if (!subjectsTarget) return
                const ok = await run(() =>
                  permissionApi.setSubjects(
                    subjectsTarget.id,
                    {
                      users: splitList(subjects.users),
                      job_titles: splitList(subjects.jobTitles),
                      roles: splitList(subjects.roles),
                    },
                    String(subjectsTarget.institution_id),
                  ),
                )
                if (ok) {
                  setSubjectsOpen(false)
                  await list.reload()
                }
              }}
            >
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={enforceOpen} onOpenChange={setEnforceOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('permissions.enforce')}</DialogTitle>
            <DialogDescription>{t('permissions.subtitle')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="enforce-institution">{t('common.institution')}</Label>
              <Select
                value={String(enforceForm.institution_id || '')}
                onValueChange={(value) => setEnforceForm({ ...enforceForm, institution_id: Number(value) })}
              >
                <SelectTrigger id="enforce-institution">
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
            </div>
            <div className="space-y-2">
              <Label htmlFor="enforce-user">{t('permissions.enforceUser')}</Label>
              <Input
                id="enforce-user"
                value={enforceForm.user}
                onChange={(event) => setEnforceForm({ ...enforceForm, user: event.target.value })}
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="enforce-resource">{t('permissions.enforceResource')}</Label>
                <Input
                  id="enforce-resource"
                  value={enforceForm.resource}
                  onChange={(event) => setEnforceForm({ ...enforceForm, resource: event.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="enforce-action">{t('permissions.enforceAction')}</Label>
                <Input
                  id="enforce-action"
                  value={enforceForm.action}
                  onChange={(event) => setEnforceForm({ ...enforceForm, action: event.target.value })}
                />
              </div>
            </div>
            {enforceResult !== null ? (
              <div className="flex items-center gap-2 border-border/70 rounded-lg border p-3 text-sm">
                <span className="text-muted-foreground">{t('permissions.enforceResult')}:</span>
                <Badge variant={enforceResult ? 'success' : 'destructive'}>
                  {enforceResult ? t('permissions.allow') : t('permissions.deny')}
                </Badge>
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEnforceOpen(false)}>
              {t('common.close')}
            </Button>
            <Button
              disabled={pending || !enforceForm.institution_id}
              onClick={async () => {
                await run(async () => {
                  const result = await permissionApi.enforce({
                    institution: String(enforceForm.institution_id),
                    user: enforceForm.user,
                    resource: enforceForm.resource,
                    action: enforceForm.action,
                  })
                  setEnforceResult(result.allowed)
                }, 'common.saved')
              }}
            >
              {t('common.confirm')}
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
          const ok = await run(
            () => permissionApi.remove(removing.id, String(removing.institution_id)),
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
