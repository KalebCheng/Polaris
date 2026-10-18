import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import type { JobLevel } from '@/shared/api/types'
import { jobLevelApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'
import { ResourceTable, type Column } from '../components/resource-table'

const emptyForm = { name: '', description: '', sort_order: '100' }

/**
 * 职级管理（Spec 第六阶段 Q4）。
 *
 * 职级**不参与授权**：它是用户属性，读取点固定三处——用户表单/详情下拉、用户列表筛选与排序、
 * 组织概览职级分布。因此本页刻意只做字典维护（名称 / 描述 / 排序 / 启停），不出现任何授权入口。
 */
export default function JobLevelsPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const institutionId = useWorkspaceInstitutionId()
  const list = useListQuery<JobLevel>((params) => jobLevelApi.list(params), { institution: institution || undefined })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<JobLevel | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [removing, setRemoving] = useState<JobLevel | null>(null)

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setDialogOpen(true)
  }

  const openEdit = (level: JobLevel) => {
    setEditing(level)
    setForm({ name: level.name, description: level.description ?? '', sort_order: String(level.sort_order ?? 100) })
    setDialogOpen(true)
  }

  const submit = async () => {
    const body = {
      name: form.name,
      description: form.description,
      sort_order: Number(form.sort_order) || 100,
      institution_id: institutionId ? Number(institutionId) : undefined,
    }
    const ok = await run(
      () => (editing ? jobLevelApi.update(editing.id, body) : jobLevelApi.create(body)),
      'common.saved',
    )
    if (ok) {
      setDialogOpen(false)
      await list.reload()
    }
  }

  const columns: Column<JobLevel>[] = [
    {
      key: 'name',
      headerKey: 'jobLevels.name',
      render: (level) => <span className="font-medium">{level.name}</span>,
    },
    { key: 'description', headerKey: 'jobLevels.description', render: (level) => level.description || '-' },
    { key: 'sort', headerKey: 'jobLevels.sortOrder', render: (level) => level.sort_order },
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (level) => (
        <RowActionsMenu
          actions={[
            { labelKey: 'common.edit', onSelect: () => openEdit(level) },
            { labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(level) },
          ]}
        />
      ),
    },
  ]

  return (
    <PageShell
      titleKey="jobLevels.title"
      subtitleKey="jobLevels.subtitle"
      actions={
        <Button onClick={openCreate}>
          <Plus className="size-4" /> {t('jobLevels.createTitle')}
        </Button>
      }
    >
      <ResourceTable<JobLevel>
        columns={columns}
        rows={list.items}
        rowKey={(level) => level.id}
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
            <DialogTitle>{editing ? t('jobLevels.editTitle') : t('jobLevels.createTitle')}</DialogTitle>
            <DialogDescription>{t('jobLevels.hint')}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <Field id="job-level-name" labelKey="jobLevels.name">
              <Input
                id="job-level-name"
                value={form.name}
                disabled={!!editing}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
              />
            </Field>
            <Field id="job-level-description" labelKey="jobLevels.description">
              <Input
                id="job-level-description"
                value={form.description}
                onChange={(event) => setForm({ ...form, description: event.target.value })}
              />
            </Field>
            <Field id="job-level-sort" labelKey="jobLevels.sortOrder" hint={t('jobLevels.sortOrderHint')}>
              <Input
                id="job-level-sort"
                type="number"
                value={form.sort_order}
                onChange={(event) => setForm({ ...form, sort_order: event.target.value })}
              />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={pending || !form.name} onClick={submit}>
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: removing?.name ?? '' }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(() => jobLevelApi.remove(removing.id), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
