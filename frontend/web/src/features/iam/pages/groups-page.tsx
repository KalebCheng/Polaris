import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import {
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
import type { Group } from '@/shared/api/types'
import { groupApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'
import { ResourceTable, type Column } from '../components/resource-table'
import { UserPicker } from '../components/user-picker'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'

const emptyForm = { name: '', display_name: '', manager: '', contact_email: '' }

/** 用户组管理：批量授权的基本单元 */
export default function GroupsPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()

  const navigate = useNavigate()
  const institutionId = useWorkspaceInstitutionId()
  const list = useListQuery<Group>((params) => groupApi.list(params), { institution: institution || undefined })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Group | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [members, setMembers] = useState<string[]>([])
  const [removing, setRemoving] = useState<Group | null>(null)

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setMembers([])
    setDialogOpen(true)
  }

  const openEdit = (group: Group) => {
    setEditing(group)
    setForm({
      name: group.name,
      display_name: group.display_name,
      manager: group.manager ?? '',
      contact_email: group.contact_email ?? '',
    })
    setMembers(group.users ?? [])
    setDialogOpen(true)
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!institution) return
    const base = {
      institution,
      name: form.name,
      display_name: form.display_name,
      manager: form.manager,
      contact_email: form.contact_email,
      is_enabled: true,
    }
    const ok = editing
      ? await run(async () => {
          await groupApi.update(editing.id, base)
          await groupApi.setMembers(editing.id, members)
        })
      : await run(async () => {
          const created = await groupApi.create(base)
          if (members.length > 0) await groupApi.setMembers(created.id, members)
        })
    if (ok) {
      setDialogOpen(false)
      await list.reload()
    }
  }

  const columns: Column<Group>[] = [
    { key: 'name', headerKey: 'groups.name', render: (group) => <span className="font-medium">{group.name}</span> },
    { key: 'display', headerKey: 'groups.displayName', render: (group) => group.display_name || '-' },
    { key: 'manager', headerKey: 'groups.manager', render: (group) => group.manager || '-' },
    { key: 'contact', headerKey: 'groups.contactEmail', render: (group) => group.contact_email || '-' },
    {
      key: 'members',
      headerKey: 'groups.memberCount',
      className: 'text-muted-foreground text-xs',
      render: (group) => group.member_count ?? (group.users ?? []).length,
    },
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (group) => (
        <RowActionsMenu
          primary={{
            labelKey: 'groups.viewDetail',
            onSelect: () => navigate(`/institutions/${institutionId}/groups/${group.id}`),
          }}
          actions={[
            { labelKey: 'common.edit', onSelect: () => openEdit(group) },
            { labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(group) },
          ]}
        />
      ),
    },
  ]

  return (
    <PageShell
      titleKey="groups.title"
      subtitleKey="groups.subtitle"
      actions={
        <Button onClick={openCreate}>
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(group) => group.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        toolbar={
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
          />
        }
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t('groups.editTitle') : t('groups.createTitle')}</DialogTitle>
            <DialogDescription>{t('groups.subtitle')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <Field id="group-name" labelKey="groups.name">
              <Input
                id="group-name"
                value={form.name}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
                disabled={Boolean(editing)}
                required
              />
            </Field>
            <Field id="group-display" labelKey="groups.displayName">
              <Input
                id="group-display"
                value={form.display_name}
                onChange={(event) => setForm({ ...form, display_name: event.target.value })}
              />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="group-manager" labelKey="groups.manager">
                <Input
                  id="group-manager"
                  value={form.manager}
                  onChange={(event) => setForm({ ...form, manager: event.target.value })}
                />
              </Field>
              <Field id="group-contact" labelKey="groups.contactEmail">
                <Input
                  id="group-contact"
                  type="email"
                  value={form.contact_email}
                  onChange={(event) => setForm({ ...form, contact_email: event.target.value })}
                />
              </Field>
            </div>
            <Field id="group-members" labelKey="groups.members" hint={t('groups.membersHint')}>
              <UserPicker value={members} onChange={setMembers} institution={institution || undefined} />
            </Field>
            {editing ? (
              <p className="text-muted-foreground text-xs">
                <Link
                  to={`/institutions/${institutionId}/groups/${editing.id}`}
                  className="text-primary hover:underline"
                >
                  {t('groups.openDetail')}
                </Link>
              </p>
            ) : null}
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
          const ok = await run(() => groupApi.remove(removing.id), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
