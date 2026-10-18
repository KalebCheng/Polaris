import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDown,
  ChevronRight,
  Info,
  Loader2,
  Network,
  Plus,
  Search,
  User,
  Users,
  X,
  type LucideIcon,
} from 'lucide-react'
import {
  Badge,
  Button,
  Checkbox,
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  EmptyState,
  Input,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import { SwitchField } from '@/shared/components/switch-field'
import { ListPagination } from '@/widgets/layout/list-pagination'
import { cn } from '@/shared/lib/utils'
import type { ApplicationAccess, ApplicationSubjectOption, ApplicationSubjectView } from '@/shared/api/types'
import { applicationApi } from '../api'
import type { ApplicationAccessFormValue } from './application-form'

/** 主体类型（与后端 subject_type 逐字一致） */
type SubjectType = 'user' | 'job_title' | 'department'

interface SubjectTypeMeta {
  key: SubjectType
  labelKey: string
  /** 覆盖语义（部门含子部门 / 职务按成员覆盖）——放在右侧面板头部，讲清"授一个等于授一片" */
  hintKey: string
  searchKey: string
  icon: LucideIcon
  /** 应用授权接口返回的三类名单字段名 */
  detailKey: 'users' | 'groups' | 'departments'
}

const SUBJECT_TYPES: SubjectTypeMeta[] = [
  {
    key: 'user',
    labelKey: 'applications.accessUsers',
    hintKey: 'applications.accessUsersHint',
    searchKey: 'applications.accessSearchUser',
    icon: User,
    detailKey: 'users',
  },
  {
    key: 'job_title',
    labelKey: 'applications.accessJobTitles',
    hintKey: 'applications.accessGroupsHint',
    searchKey: 'applications.accessSearchGroup',
    icon: Users,
    detailKey: 'groups',
  },
  {
    key: 'department',
    labelKey: 'applications.accessDepartments',
    hintKey: 'applications.accessDepartmentsHint',
    searchKey: 'applications.accessSearchDepartment',
    icon: Network,
    detailKey: 'departments',
  },
]

/** 从候选里勾选出来的待授权主体 */
interface PickedSubject {
  type: SubjectType
  name: string
  display_name: string
  path?: string
}

type AuthorizedMap = Record<SubjectType, string[]>

function emptyAuthorized(): AuthorizedMap {
  return { user: [], job_title: [], department: [] }
}

interface ApplicationAccessEditorProps {
  access: ApplicationAccessFormValue
  onChange: (next: ApplicationAccessFormValue) => void
  /** 已保存的授权详情：提供展示名 / 层级路径 / 「已失效」标记 */
  detail?: ApplicationAccess | null
}

/**
 * 应用访问授权编辑器（向导第 6 步）。
 *
 * 交互形态对齐阿里云 IDaaS「应用授权」：**左侧授权类型（含各自数量）、右侧该类型的授权名单表格**
 * ——原先三个下拉多选框把"已授权了谁"整体藏在控件里，既看不到全局、也没法逐条核对与移除。
 *
 * 顶部只有「全员可访问」一个开关；关闭后进入受限模式。名单为空**不阻止保存**（Spec Q4），
 * 空名单由右侧空态与左侧计数表达，不额外堆解释性文案。
 */
export function ApplicationAccessEditor({ access, onChange, detail }: ApplicationAccessEditorProps) {
  const { t } = useTranslation()
  const restricted = access.mode === 'restricted'

  const [activeType, setActiveType] = useState<SubjectType>('user')
  /** 已授权名单的本地筛选（按名称/标识/路径） */
  const [filter, setFilter] = useState('')
  const [pickerOpen, setPickerOpen] = useState(false)
  /** 本次会话刚选进来的主体：保存前靠它显示成人类可读名称 */
  const [picked, setPicked] = useState<Record<string, ApplicationSubjectView>>({})

  const meta = SUBJECT_TYPES.find((item) => item.key === activeType) ?? SUBJECT_TYPES[0]
  const selected = access[meta.detailKey]

  const counts = useMemo(
    () => ({
      user: access.users.length,
      job_title: access.groups.length,
      department: access.departments.length,
    }),
    [access.users.length, access.groups.length, access.departments.length],
  )

  /** 已授权条目：接口返回的展示信息（含"已失效"标记）→ 本次刚选中的 → 退回标识名 */
  const rows: ApplicationSubjectView[] = useMemo(() => {
    const known = new Map<string, ApplicationSubjectView>(
      (detail?.[meta.detailKey] ?? []).map((item) => [item.name, item]),
    )
    for (const [name, item] of Object.entries(picked)) {
      if (!known.has(name)) known.set(name, item)
    }
    return selected.map((name) => known.get(name) ?? { name, display_name: name, missing: false })
  }, [detail, meta.detailKey, picked, selected])

  const filteredRows = useMemo(() => {
    const keyword = filter.trim().toLowerCase()
    if (!keyword) return rows
    return rows.filter((row) =>
      [row.name, row.display_name, row.path ?? ''].some((field) => field.toLowerCase().includes(keyword)),
    )
  }, [filter, rows])

  /**
   * 批量加入：把弹窗里勾选的主体并入名单（按类型分别追加），并记下展示信息
   * ——保存前也能显示成展示名，而不是裸标识。
   */
  const mergePicked = useCallback(
    (items: PickedSubject[]) => {
      if (items.length === 0) return
      setPicked((prev) => {
        const next = { ...prev }
        for (const item of items) {
          next[item.name] = {
            name: item.name,
            display_name: item.display_name,
            path: item.path,
            missing: false,
          }
        }
        return next
      })
      const add = (current: string[], type: SubjectType) =>
        Array.from(new Set([...current, ...items.filter((item) => item.type === type).map((item) => item.name)]))
      onChange({
        mode: access.mode,
        users: add(access.users, 'user'),
        groups: add(access.groups, 'job_title'),
        departments: add(access.departments, 'department'),
      })
    },
    [access, onChange],
  )

  const remove = useCallback(
    (name: string, type: SubjectTypeMeta) => {
      onChange({
        mode: access.mode,
        users: type.key === 'user' ? access.users.filter((item) => item !== name) : access.users,
        groups:
          type.key === 'job_title' ? access.groups.filter((item) => item !== name) : access.groups,
        departments:
          type.key === 'department' ? access.departments.filter((item) => item !== name) : access.departments,
      })
    },
    [access, onChange],
  )

  return (
    <div className="space-y-4">
      <SwitchField
        labelKey="applications.accessAll"
        descriptionKey="applications.accessAllHint"
        checked={!restricted}
        onCheckedChange={(checked) => onChange({ ...access, mode: checked ? 'all' : 'restricted' })}
      />

      {restricted ? (
        <div className="grid gap-4 lg:grid-cols-[minmax(0,200px)_minmax(0,1fr)]">
          {/* 左：授权类型（各类型已授权数量一眼可见） */}
          <div className="border-border/70 h-fit rounded-xl border p-2">
            <p className="text-muted-foreground px-2 pt-1 pb-2 text-xs font-medium">
              {t('applications.accessTypes')}
            </p>
            <div className="space-y-1">
              {SUBJECT_TYPES.map((item) => (
                <AccessTypeButton
                  key={item.key}
                  meta={item}
                  active={item.key === activeType}
                  count={counts[item.key]}
                  onSelect={() => {
                    setActiveType(item.key)
                    setFilter('')
                  }}
                />
              ))}
            </div>
          </div>

          {/* 右：该类型的授权名单 */}
          <div className="border-border/70 rounded-xl border">
            <div className="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
              <Button type="button" size="sm" onClick={() => setPickerOpen(true)}>
                <Plus className="size-4" /> {t('applications.accessAdd')}
              </Button>
              <Input
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                placeholder={t(meta.searchKey)}
                aria-label={t(meta.searchKey)}
                className="h-9 max-w-56"
              />
              <span className="text-muted-foreground ml-auto text-xs">{t(meta.hintKey)}</span>
            </div>

            {rows.length === 0 ? (
              <EmptyState
                icon={meta.icon}
                title={t('applications.accessListEmpty')}
                description={t('applications.accessListEmptyHint')}
              />
            ) : filteredRows.length === 0 ? (
              <EmptyState title={t('applications.accessNoMatch')} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t(meta.labelKey)}</TableHead>
                    <TableHead>{t('applications.accessIdentifier')}</TableHead>
                    {meta.key !== 'user' ? <TableHead>{t('applications.accessPath')}</TableHead> : null}
                    <TableHead className="w-16 text-right">{t('common.actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredRows.map((row) => (
                    <TableRow key={row.name}>
                      <TableCell className="max-w-56 truncate">
                        <span className="flex items-center gap-2">
                          <span className="truncate">{row.display_name || row.name}</span>
                          {row.missing ? (
                            <Badge variant="outline">{t('applications.accessMissing')}</Badge>
                          ) : null}
                        </span>
                      </TableCell>
                      <TableCell className="text-muted-foreground font-mono text-xs">{row.name}</TableCell>
                      {meta.key !== 'user' ? (
                        <TableCell className="text-muted-foreground max-w-56 truncate text-xs">
                          {row.path || '-'}
                        </TableCell>
                      ) : null}
                      <TableCell className="text-right">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label={t('applications.accessRemove', { name: row.display_name || row.name })}
                          onClick={() => remove(row.name, meta)}
                        >
                          <X className="size-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </div>
        </div>
      ) : null}

      <AccessPickerDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        initialType={activeType}
        authorized={{
          user: access.users,
          job_title: access.groups,
          department: access.departments,
        }}
        onConfirm={mergePicked}
      />
    </div>
  )
}

/** 左侧授权类型条目（外层名单与弹窗共用同一形态：图标 + 名称 + 计数） */
function AccessTypeButton({
  meta,
  active,
  count,
  onSelect,
}: {
  meta: SubjectTypeMeta
  active: boolean
  count?: number
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const Icon = meta.icon
  return (
    <button
      type="button"
      aria-current={active ? 'true' : undefined}
      onClick={onSelect}
      className={cn(
        'flex w-full items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] transition-colors',
        active ? 'bg-primary/10 text-primary font-medium' : 'hover:bg-accent/60',
      )}
    >
      <span className="flex min-w-0 items-center gap-2">
        <Icon className="size-4 shrink-0" />
        <span className="truncate">{t(meta.labelKey)}</span>
      </span>
      {count === undefined ? null : (
        <span aria-hidden className="text-muted-foreground text-xs tabular-nums">
          {count}
        </span>
      )}
    </button>
  )
}

interface AccessPickerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 打开时默认停在哪个类型（= 外层正在看的那一类，避免每次都要重新切） */
  initialType: SubjectType
  /** 已授权名单：候选里排除它们（已授权的在名单里管理，不重复出现） */
  authorized: AuthorizedMap
  onConfirm: (picked: PickedSubject[]) => void
}

/** 扁平候选每页条数（分页器） */
const PAGE_SIZE = 10
/** 部门树每层每次加载的条数（「加载更多」按此递增） */
const TREE_PAGE = 10
/** 服务端候选上限 */
const CANDIDATE_MAX = 200

/**
 * 「添加授权」弹窗（交互对齐 IDaaS）：
 * 左侧在弹窗内切换授权类型，右侧是该类型的候选选择区——**勾选式**，
 * 勾选可跨类型累积（「已选 N 项」），底部一次性确定。
 *
 * 右侧三种形态：
 *   - 用户 / 职务：扁平分页表格（每页 10 条 + 分页器）；
 *   - 部门 + 关键词：扁平搜索结果（跨层级，带层级路径，也分页）；
 *   - 部门 + 无关键词：**按层加载的部门树**（展开某节点时才取它的下一层，
 *     顶层与每层都提供「加载更多」）——大组织的部门树不可能一次拉全。
 */
function AccessPickerDialog({ open, onOpenChange, initialType, authorized, onConfirm }: AccessPickerDialogProps) {
  const { t } = useTranslation()
  const [activeType, setActiveType] = useState<SubjectType>(initialType)
  // 用 ref 读"最新初始类型"：effect 只在**打开**时跑一次，不因它变化重置用户已勾选的内容
  const initialTypeRef = useRef(initialType)
  initialTypeRef.current = initialType

  const [draft, setDraft] = useState<AuthorizedMap>(emptyAuthorized)
  /** 勾选项的展示信息（跨类型累积，确定时回传给名单） */
  const [draftInfo, setDraftInfo] = useState<Record<string, ApplicationSubjectOption & { type: SubjectType }>>({})
  const [keyword, setKeyword] = useState('')
  const [showSelected, setShowSelected] = useState(false)

  // 扁平分页（用户 / 职务 / 部门搜索）
  const [page, setPage] = useState(1)
  const [flat, setFlat] = useState<{ items: ApplicationSubjectOption[]; total: number }>({ items: [], total: 0 })
  // 部门树（顶层 + 逐层懒加载）
  const [roots, setRoots] = useState<{ items: ApplicationSubjectOption[]; total: number }>({ items: [], total: 0 })
  const [rootLimit, setRootLimit] = useState(TREE_PAGE)
  const [children, setChildren] = useState<Record<number, ApplicationSubjectOption[]>>({})
  const [childrenTotal, setChildrenTotal] = useState<Record<number, number>>({})
  const [childrenLimit, setChildrenLimit] = useState<Record<number, number>>({})
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const [loading, setLoading] = useState(false)

  const meta = SUBJECT_TYPES.find((item) => item.key === activeType) ?? SUBJECT_TYPES[0]
  const searching = keyword.trim() !== ''
  const treeMode = activeType === 'department' && !searching

  // 每次打开都从"零勾选"开始，并停在调用方正在查看的类型
  useEffect(() => {
    if (!open) return
    setActiveType(initialTypeRef.current)
    setDraft(emptyAuthorized())
    setDraftInfo({})
    setKeyword('')
    setPage(1)
    setShowSelected(false)
    setExpanded({})
    setChildren({})
    setChildrenTotal({})
    setChildrenLimit({})
    setRootLimit(TREE_PAGE)
  }, [open])

  // 扁平候选：关键词防抖 300ms + 分页
  useEffect(() => {
    if (!open || treeMode) return
    let cancelled = false
    setLoading(true)
    const timer = window.setTimeout(() => {
      void applicationApi
        .subjectOptions({
          type: activeType,
          keyword: keyword.trim(),
          offset: (page - 1) * PAGE_SIZE,
          limit: PAGE_SIZE,
        })
        .then((result) => {
          if (!cancelled) setFlat({ items: result.items ?? [], total: result.total ?? 0 })
        })
        .catch(() => {
          if (!cancelled) setFlat({ items: [], total: 0 })
        })
        .finally(() => {
          if (!cancelled) setLoading(false)
        })
    }, 300)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [open, treeMode, activeType, keyword, page])

  // 部门树顶层（打开 / 切到部门 / 点「加载更多」时增加层内条数）
  useEffect(() => {
    if (!open || !treeMode) return
    let cancelled = false
    setLoading(true)
    void applicationApi
      .subjectOptions({ type: 'department', offset: 0, limit: rootLimit })
      .then((result) => {
        if (!cancelled) setRoots({ items: result.items ?? [], total: result.total ?? 0 })
      })
      .catch(() => {
        if (!cancelled) setRoots({ items: [], total: 0 })
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, treeMode, rootLimit])

  /** 展开某节点：第一次展开时懒加载它的下一层 */
  const loadChildren = useCallback((parentId: number, limit: number) => {
    setLoading(true)
    void applicationApi
      .subjectOptions({ type: 'department', parentId, offset: 0, limit })
      .then((result) => {
        setChildren((prev) => ({ ...prev, [parentId]: result.items ?? [] }))
        setChildrenTotal((prev) => ({ ...prev, [parentId]: result.total ?? 0 }))
      })
      .catch(() => {
        setChildren((prev) => ({ ...prev, [parentId]: [] }))
      })
      .finally(() => setLoading(false))
  }, [])

  const toggleNode = (item: ApplicationSubjectOption) => {
    const id = item.id ?? 0
    const next = !expanded[id]
    setExpanded((prev) => ({ ...prev, [id]: next }))
    if (next && children[id] === undefined) loadChildren(id, childrenLimit[id] ?? TREE_PAGE)
  }

  const moreChildren = (id: number) => {
    const next = (childrenLimit[id] ?? TREE_PAGE) + TREE_PAGE
    setChildrenLimit((prev) => ({ ...prev, [id]: next }))
    loadChildren(id, next)
  }

  const selectable = flat.items.filter((item) => !authorized[activeType].includes(item.name))
  const selectedNames = draft[activeType]
  const totalSelected = draft.user.length + draft.job_title.length + draft.department.length
  const allSelected = selectable.length > 0 && selectable.every((item) => selectedNames.includes(item.name))
  const someSelected = selectable.some((item) => selectedNames.includes(item.name))

  const toggle = (item: ApplicationSubjectOption, checked: boolean) => {
    setDraft((prev) => ({
      ...prev,
      [activeType]: checked
        ? Array.from(new Set([...prev[activeType], item.name]))
        : prev[activeType].filter((name) => name !== item.name),
    }))
    setDraftInfo((prev) => {
      if (!checked) {
        const next = { ...prev }
        delete next[item.name]
        return next
      }
      return { ...prev, [item.name]: { ...item, type: activeType } }
    })
  }

  const toggleAll = (checked: boolean) => {
    setDraft((prev) => ({ ...prev, [activeType]: checked ? selectable.map((item) => item.name) : [] }))
    setDraftInfo((prev) => {
      const next = { ...prev }
      for (const item of selectable) {
        if (checked) next[item.name] = { ...item, type: activeType }
        else delete next[item.name]
      }
      return next
    })
  }

  const unselect = (name: string, type: SubjectType) => {
    setDraft((prev) => ({ ...prev, [type]: prev[type].filter((item) => item !== name) }))
    setDraftInfo((prev) => {
      const next = { ...prev }
      delete next[name]
      return next
    })
  }

  const confirm = () => {
    const picked: PickedSubject[] = []
    for (const type of ['user', 'job_title', 'department'] as SubjectType[]) {
      for (const name of draft[type]) {
        const info = draftInfo[name]
        picked.push({ type, name, display_name: info?.display_name || name, path: info?.path })
      }
    }
    onConfirm(picked)
    onOpenChange(false)
  }

  /** 候选行：勾选框 + 名称（部门树按 depth 缩进，已授权的禁用并标注） */
  const renderRow = (
    item: ApplicationSubjectOption,
    options: { depth?: number; toggleNode?: () => void; expanded?: boolean } = {},
  ) => {
    const rowAuthorized = authorized[activeType].includes(item.name)
    const depth = options.depth ?? 0
    return (
      <div
        key={item.name}
        className="border-border/60 flex items-center gap-2 border-b py-1"
        style={{ paddingLeft: `${depth * 16}px` }}
      >
        {options.toggleNode ? (
          item.has_child ? (
            <button
              type="button"
              className="text-muted-foreground flex size-6 shrink-0 items-center justify-center"
              aria-label={
                options.expanded
                  ? t('applications.accessCollapseNode', { name: item.display_name || item.name })
                  : t('applications.accessExpandNode', { name: item.display_name || item.name })
              }
              onClick={options.toggleNode}
            >
              {options.expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
            </button>
          ) : (
            // 无下级：留出与展开按钮等宽的占位，保持同行名称左对齐
            <span className="inline-block size-6 shrink-0" />
          )
        ) : null}
        <Checkbox
          aria-label={t('applications.accessSelectRow', { name: item.display_name || item.name })}
          checked={rowAuthorized || selectedNames.includes(item.name)}
          disabled={rowAuthorized}
          onCheckedChange={(value) => toggle(item, value === true)}
        />
        <span className="min-w-0 flex-1 truncate text-[13px]">{item.display_name || item.name}</span>
        {rowAuthorized ? <Badge variant="outline">{t('applications.accessAlreadyAuthorized')}</Badge> : null}
        {!treeMode ? (
          <>
            <span className="text-muted-foreground w-40 shrink-0 truncate font-mono text-[11px]">
              {item.name}
            </span>
            {meta.key !== 'user' ? (
              <span className="text-muted-foreground w-48 shrink-0 truncate text-[11px]">
                {item.path || '-'}
              </span>
            ) : null}
          </>
        ) : null}
      </div>
    )
  }

  /** 部门树节点（含已加载的下级） */
  const renderNode = (item: ApplicationSubjectOption, depth: number) => {
    const id = item.id ?? 0
    const isOpen = Boolean(expanded[id])
    const rows = [renderRow(item, { depth, expanded: isOpen, toggleNode: () => toggleNode(item) })]
    if (isOpen) {
      for (const child of children[id] ?? []) rows.push(...renderNode(child, depth + 1))
      const loaded = (children[id] ?? []).length
      if (loaded > 0 && loaded < (childrenTotal[id] ?? 0) && loaded < CANDIDATE_MAX) {
        rows.push(
          <div key={`more-${id}`} className="py-2 pl-6">
            <Button type="button" variant="outline" size="sm" onClick={() => moreChildren(id)}>
              {t('applications.accessLoadMore')}
            </Button>
          </div>,
        )
      }
    }
    return rows
  }

  const emptyCandidates = treeMode ? roots.items.length === 0 : selectable.length === 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-4xl"
        innerClassName="p-0 gap-0 overflow-hidden grid-rows-[auto_auto_minmax(0,1fr)_auto]"
      >
        <DialogHeader className="border-b px-6 py-4">
          <DialogTitle>{t('applications.accessAddTitle')}</DialogTitle>
        </DialogHeader>

        {/* 一行后果说明（对齐 IDaaS 的提示条） */}
        <div className="bg-muted/50 text-muted-foreground border-b px-6 py-2 text-xs">
          <span className="flex items-center gap-2">
            <Info className="size-3.5 shrink-0" />
            {t('applications.accessAddBanner')}
          </span>
        </div>

        <div className="grid min-h-0 grid-cols-1 overflow-hidden lg:grid-cols-[minmax(0,168px)_minmax(0,1fr)]">
          {/* 左：授权类型（在弹窗内切换，可跨类型累积勾选） */}
          <div className="border-border/60 border-b p-3 lg:border-r lg:border-b-0">
            <p className="text-muted-foreground px-2 pt-1 pb-2 text-xs font-medium">
              {t('applications.accessTypes')}
            </p>
            <div className="space-y-1">
              {SUBJECT_TYPES.map((item) => (
                <AccessTypeButton
                  key={item.key}
                  meta={item}
                  active={item.key === activeType}
                  onSelect={() => {
                    setActiveType(item.key)
                    setKeyword('')
                    setPage(1)
                  }}
                />
              ))}
            </div>
          </div>

          {/* 右：候选选择区 */}
          <div className="flex min-h-0 flex-col">
            <div className="flex items-center gap-2 px-4 pt-3 pb-2">
              <span className="text-[13px] font-medium">{t(meta.labelKey)}</span>
              <span className="text-muted-foreground ml-auto text-xs">
                {t('applications.accessSelected', { count: totalSelected })}
              </span>
              <button
                type="button"
                className="text-primary text-xs hover:underline"
                onClick={() => setShowSelected((prev) => !prev)}
              >
                {t(showSelected ? 'applications.accessCollapse' : 'applications.accessExpand')}
              </button>
            </div>

            {showSelected && totalSelected > 0 ? (
              <div className="flex flex-wrap gap-1.5 px-4 pb-2">
                {(['user', 'job_title', 'department'] as SubjectType[]).flatMap((type) =>
                  draft[type].map((name) => (
                    <Badge key={`${type}:${name}`} variant="secondary">
                      {draftInfo[name]?.display_name || name}
                      <button
                        type="button"
                        aria-label={t('applications.accessUnselect', { name: draftInfo[name]?.display_name || name })}
                        className="ml-1 opacity-60 hover:opacity-100"
                        onClick={() => unselect(name, type)}
                      >
                        <X className="size-3" />
                      </button>
                    </Badge>
                  )),
                )}
              </div>
            ) : null}

            <div className="px-4 pb-2">
              <div className="relative">
                <Search className="text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2" />
                <Input
                  className="h-9 pl-9"
                  value={keyword}
                  onChange={(event) => {
                    setKeyword(event.target.value)
                    setPage(1)
                  }}
                  placeholder={t(meta.searchKey)}
                  aria-label={t(meta.searchKey)}
                />
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-3">
              {loading && (treeMode ? roots.items.length === 0 : flat.items.length === 0) ? (
                <p className="text-muted-foreground flex items-center gap-2 px-1 py-6 text-xs">
                  <Loader2 className="size-3.5 animate-spin" /> {t('common.loading')}
                </p>
              ) : emptyCandidates ? (
                <EmptyState title={t('applications.accessCandidateEmpty')} />
              ) : treeMode ? (
                <>
                  {roots.items.map((item) => renderNode(item, 0))}
                  {roots.items.length < roots.total && roots.items.length < CANDIDATE_MAX ? (
                    <div className="py-3 text-center">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setRootLimit((prev) => Math.min(prev + TREE_PAGE, CANDIDATE_MAX))}
                      >
                        {t('applications.accessLoadMore')}
                      </Button>
                    </div>
                  ) : null}
                </>
              ) : (
                <>
                  {/* 表头：全选当前列出的候选 */}
                  <div className="border-border/70 flex items-center gap-2 border-b py-1.5">
                    <span className="flex size-6 shrink-0 items-center justify-center">
                      <Checkbox
                        aria-label={t('applications.accessSelectAll')}
                        checked={allSelected ? true : someSelected ? 'indeterminate' : false}
                        onCheckedChange={(value) => toggleAll(value === true)}
                      />
                    </span>
                    {renderColumnsHeader(meta, t)}
                  </div>
                  {selectable.map((item) => renderRow(item))}
                  <div className="pt-3">
                    <ListPagination page={page} size={PAGE_SIZE} total={flat.total} onPageChange={setPage} />
                  </div>
                </>
              )}
            </div>
          </div>
        </div>

        <DialogFooter className="border-t px-6 py-3">
          <DialogClose asChild>
            <Button type="button" variant="outline">
              {t('common.cancel')}
            </Button>
          </DialogClose>
          <Button type="button" disabled={totalSelected === 0} onClick={confirm}>
            {t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** 扁平候选的表头列名（与 renderRow 的列一一对应） */
function renderColumnsHeader(meta: SubjectTypeMeta, t: (key: string) => string) {
  return (
    <>
      <span className="min-w-0 flex-1 text-xs font-medium">{t(meta.labelKey)}</span>
      <span className="text-muted-foreground w-40 shrink-0 text-xs font-medium">
        {t('applications.accessIdentifier')}
      </span>
      {meta.key !== 'user' ? (
        <span className="text-muted-foreground w-48 shrink-0 text-xs font-medium">
          {t('applications.accessPath')}
        </span>
      ) : null}
    </>
  )
}
