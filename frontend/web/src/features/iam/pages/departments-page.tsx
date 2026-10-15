import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight, ChevronUp, Network, Pencil, Plus, Trash2, Users } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Combobox,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  EmptyState,
  Input,
  Skeleton,
  Textarea,
  type SelectOption,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import type { Department, User } from '@/shared/api/types'
import { departmentApi } from '../api'
import { UserPicker } from '../components/user-picker'
import { useWorkspaceInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'

interface DepartmentForm {
  name: string
  display_name: string
  description: string
  parent_id: number | null
}

const emptyForm: DepartmentForm = { name: '', display_name: '', description: '', parent_id: null }

/** 树节点（含展开态与子节点） */
interface TreeNode extends Department {
  children: TreeNode[]
}

/**
 * 部门管理：租户内的部门树（左树 + 右详情/成员）。
 * 支持换父级移动（编辑上级部门）与同级上下移动（排序）。
 * 部门仅用于用户归属与筛选，不参与权限判定（见 Spec ADR D2）。
 */
export default function DepartmentsPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()

  const list = useListQuery<Department>((params) => departmentApi.list(params), {
    institution: institution || undefined,
    size: 200,
  })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Department | null>(null)
  const [form, setForm] = useState<DepartmentForm>(emptyForm)
  const [removing, setRemoving] = useState<Department | null>(null)
  const [selected, setSelected] = useState<Department | null>(null)
  const [collapsed, setCollapsed] = useState<Set<number>>(new Set())
  const [members, setMembers] = useState<User[]>([])
  const [membersLoading, setMembersLoading] = useState(false)

  // 构建树：按 parent_id 归并，父先于子（后端已按 level 排序）
  const tree = useMemo(() => buildTree(list.items), [list.items])
  // 记录每个节点的同级序号，用于上/下移动按钮的可用性
  const siblingIndex = useMemo(() => indexSiblings(list.items), [list.items])

  const parentOptions: SelectOption[] = useMemo(
    () =>
      list.items
        .filter((d) => d.name !== editing?.name)
        .map((d) => ({ value: String(d.id), label: d.name_path || d.name })),
    [list.items, editing],
  )

  // 选中项默认取第一个根部门
  useEffect(() => {
    if (!selected && list.items.length > 0) {
      setSelected(list.items.find((d) => d.parent_id == null) ?? list.items[0])
    }
  }, [list.items, selected])

  // 成员编辑（"添加 / 移出成员"）：整体替换该部门的成员集合
  const [membersDialogOpen, setMembersDialogOpen] = useState(false)
  const [pendingMembers, setPendingMembers] = useState<string[]>([])

  // 载入选中部门的成员
  const loadMembers = useCallback(
    async (dept: Department | null) => {
      if (!dept || !institution) {
        setMembers([])
        return
      }
      setMembersLoading(true)
      try {
        const result = await departmentApi.members(dept.id, institution)
        setMembers(result.items ?? [])
      } catch {
        setMembers([])
      } finally {
        setMembersLoading(false)
      }
    },
    [institution],
  )

  useEffect(() => {
    void loadMembers(selected)
  }, [selected, loadMembers])

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setDialogOpen(true)
  }

  const openCreateChild = (parent: Department) => {
    setEditing(null)
    setForm({ ...emptyForm, parent_id: parent.id })
    setDialogOpen(true)
  }

  const openEdit = (dept: Department) => {
    setEditing(dept)
    setForm({
      name: dept.name,
      display_name: dept.display_name,
      description: dept.description ?? '',
      parent_id: dept.parent_id ?? null,
    })
    setDialogOpen(true)
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!institution) return
    const base = {
      institution,
      name: form.name,
      display_name: form.display_name,
      description: form.description,
      parent_id: form.parent_id,
    }
    const ok = editing
      ? await run(() => departmentApi.update(editing.id, base))
      : await run(() => departmentApi.create(base))
    if (ok) {
      setDialogOpen(false)
      await list.reload()
    }
  }

  const toggle = (id: number) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const move = async (dept: Department, direction: 'up' | 'down') => {
    const ok = await run(() => departmentApi.reorder(dept.id, direction))
    if (ok) await list.reload()
  }

  const renderNode = (node: TreeNode, depth: number): React.ReactNode => {
    const isOpen = !collapsed.has(node.id)
    const hasChildren = node.children.length > 0
    const index = siblingIndex.get(node.id) ?? { index: 0, total: 1 }
    const isSelected = selected?.id === node.id
    return (
      <div key={node.id}>
        <div
          className={`group flex items-center gap-1 rounded-md pr-1 ${isSelected ? 'bg-accent' : 'hover:bg-muted/60'}`}
          style={{ paddingLeft: `${depth * 16}px` }}
        >
          <button
            type="button"
            className="flex size-6 shrink-0 items-center justify-center text-muted-foreground"
            onClick={() => hasChildren && toggle(node.id)}
            aria-label={hasChildren ? 'toggle' : ''}
          >
            {hasChildren ? (
              isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />
            ) : (
              <span className="inline-block size-4" />
            )}
          </button>
          <button
            type="button"
            className="min-w-0 flex-1 truncate py-1.5 text-left text-sm"
            onClick={() => setSelected(node)}
          >
            {node.display_name || node.name}
          </button>
          <div className="hidden shrink-0 items-center gap-0.5 group-hover:flex">
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              disabled={index.index <= 0}
              onClick={() => void move(node, 'up')}
              aria-label={t('departments.moveUp')}
            >
              <ChevronUp className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              disabled={index.index >= index.total - 1}
              onClick={() => void move(node, 'down')}
              aria-label={t('departments.moveDown')}
            >
              <ChevronDown className="size-3.5" />
            </Button>
            <Button variant="ghost" size="icon" className="size-6" onClick={() => openCreateChild(node)} aria-label={t('departments.addChild')}>
              <Plus className="size-3.5" />
            </Button>
            <Button variant="ghost" size="icon" className="size-6" onClick={() => openEdit(node)} aria-label={t('common.edit')}>
              <Pencil className="size-3.5" />
            </Button>
            <Button variant="ghost" size="icon" className="size-6" onClick={() => setRemoving(node)} aria-label={t('common.delete')}>
              <Trash2 className="size-3.5" />
            </Button>
          </div>
        </div>
        {hasChildren && isOpen ? node.children.map((child) => renderNode(child, depth + 1)) : null}
      </div>
    )
  }

  return (
    <PageShell
      titleKey="departments.title"
      subtitleKey="departments.subtitle"
      actions={
        <Button onClick={openCreate}>
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        {/* 左：部门树 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('departments.treeTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <ListToolbar keyword={list.keyword} onKeywordChange={list.setKeyword} onSubmit={list.search} />
            {list.loading ? (
              <div className="space-y-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-8 rounded-md" />
                ))}
              </div>
            ) : list.items.length === 0 ? (
              <EmptyState icon={Network} title={t('departments.empty')} description={t('departments.emptyHint')} />
            ) : (
              <div className="max-h-[28rem] overflow-auto">{tree.map((node) => renderNode(node, 0))}</div>
            )}
          </CardContent>
        </Card>

        {/* 右：部门详情 + 成员 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{selected ? selected.display_name || selected.name : t('departments.detailTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {!selected ? (
              <EmptyState icon={Network} title={t('departments.selectHint')} />
            ) : (
              <>
                <dl className="grid grid-cols-2 gap-2 text-sm">
                  <dt className="text-muted-foreground">{t('departments.name')}</dt>
                  <dd className="font-mono">{selected.name}</dd>
                  <dt className="text-muted-foreground">{t('departments.path')}</dt>
                  <dd className="truncate">{selected.name_path || '-'}</dd>
                  <dt className="text-muted-foreground">{t('departments.level')}</dt>
                  <dd>{selected.level}</dd>
                  <dt className="text-muted-foreground">{t('departments.description')}</dt>
                  <dd className="col-span-1 sm:col-span-1">{selected.description || '-'}</dd>
                </dl>
                <div className="flex items-center justify-between gap-2">
                  <span className="flex items-center gap-2 text-sm font-medium">
                    <Users className="size-4" /> {t('departments.members')}
                    <Badge variant="secondary">{members.length}</Badge>
                  </span>
                  <span className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setPendingMembers(members.map((user) => user.name))
                        setMembersDialogOpen(true)
                      }}
                    >
                      <Users className="size-3.5" /> {t('departments.manageMembers')}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => openEdit(selected)}>
                      <Pencil className="size-3.5" /> {t('common.edit')}
                    </Button>
                  </span>
                </div>
                {membersLoading ? (
                  <Skeleton className="h-20 rounded-md" />
                ) : members.length === 0 ? (
                  <p className="text-muted-foreground text-sm">{t('departments.noMembers')}</p>
                ) : (
                  <ul className="divide-y rounded-md border">
                    {members.map((user) => (
                      <li key={user.id} className="flex items-center justify-between px-3 py-2 text-sm">
                        <span className="truncate">{user.display_name || user.name}</span>
                        <span className="text-muted-foreground truncate">{user.email || user.name}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </>
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t('departments.editTitle') : t('departments.createTitle')}</DialogTitle>
            <DialogDescription>{t('departments.subtitle')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <Field id="dept-name" labelKey="departments.name">
              <Input
                id="dept-name"
                value={form.name}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
                disabled={Boolean(editing)}
                required
              />
            </Field>
            <Field id="dept-display" labelKey="departments.displayName">
              <Input
                id="dept-display"
                value={form.display_name}
                onChange={(event) => setForm({ ...form, display_name: event.target.value })}
              />
            </Field>
            <Field id="dept-parent" labelKey="departments.parent" hint={t('departments.parentHint')}>
              <Combobox
                options={parentOptions}
                value={form.parent_id != null ? String(form.parent_id) : ''}
                onChange={(value) => setForm({ ...form, parent_id: value ? Number(value) : null })}
                placeholder={t('departments.parentPlaceholder')}
              />
            </Field>
            <Field id="dept-desc" labelKey="departments.description">
              <Textarea
                id="dept-desc"
                value={form.description}
                onChange={(event) => setForm({ ...form, description: event.target.value })}
                rows={2}
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

      <Dialog open={membersDialogOpen} onOpenChange={setMembersDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('departments.manageMembersTitle')}</DialogTitle>
            <DialogDescription>{t('departments.manageMembersHint')}</DialogDescription>
          </DialogHeader>
          <Field id="department-members" labelKey="departments.members" hint={t('departments.membersEffectHint')}>
            <UserPicker
              value={pendingMembers}
              onChange={setPendingMembers}
              institution={institution || undefined}
            />
          </Field>
          <DialogFooter>
            <Button variant="outline" onClick={() => setMembersDialogOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              disabled={pending || !selected}
              onClick={async () => {
                if (!selected) return
                const ok = await run(async () => {
                  await departmentApi.setMembers(selected.id, pendingMembers)
                  await loadMembers(selected)
                  await list.reload()
                })
                if (ok) setMembersDialogOpen(false)
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
          const ok = await run(() => departmentApi.remove(removing.id), 'common.deleted')
          if (ok) {
            if (selected?.id === removing.id) setSelected(null)
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}

/** 按 parent_id 组装树（保持后端顺序：level 升序、同层 sort_index 升序）。 */
function buildTree(items: Department[]): TreeNode[] {
  const nodes = new Map<number, TreeNode>()
  for (const item of items) {
    nodes.set(item.id, { ...item, children: [] })
  }
  const roots: TreeNode[] = []
  for (const item of items) {
    const node = nodes.get(item.id)!
    if (item.parent_id != null && nodes.has(item.parent_id)) {
      nodes.get(item.parent_id)!.children.push(node)
    } else {
      roots.push(node)
    }
  }
  return roots
}

/** 计算每个节点在其同级内的序号（用于上/下移动按钮可用性）。 */
function indexSiblings(items: Department[]): Map<number, { index: number; total: number }> {
  const groups = new Map<string, Department[]>()
  for (const item of items) {
    const key = item.parent_id == null ? 'root' : String(item.parent_id)
    const arr = groups.get(key) ?? []
    arr.push(item)
    groups.set(key, arr)
  }
  const result = new Map<number, { index: number; total: number }>()
  for (const arr of groups.values()) {
    const total = arr.length
    arr.forEach((item, index) => result.set(item.id, { index, total }))
  }
  return result
}
