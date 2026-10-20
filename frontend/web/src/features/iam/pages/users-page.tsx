import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Badge, Button } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import { UserAvatar } from '@/shared/components/user-avatar'
import { formatRelative } from '@/shared/lib/format'
import type { Department, JobLevel, User } from '@/shared/api/types'
import { departmentApi, jobLevelApi, userApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { useOnceKey } from '../hooks/use-once-key'
import { ResourceTable, type Column } from '../components/resource-table'

/**
 * 用户管理：目录列表 + 深操作入口。
 *
 * 分工：列表只做"找到人 + 快捷动作"，**资料编辑与全部安全/授权动作都在用户详情页**
 * （`/institutions/:inst/users/:id`）——这样列表页不会再膨胀成"六个弹窗的合集"。
 */
export default function UsersPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const institutionId = useWorkspaceInstitutionId()
  const navigate = useNavigate()

  const [status, setStatus] = useState('')
  const [mfaFilter, setMfaFilter] = useState('')
  const [departmentFilter, setDepartmentFilter] = useState('')
  const [jobLevelFilter, setJobLevelFilter] = useState('')
  const [departments, setDepartments] = useState<Department[]>([])
  const [jobLevels, setJobLevels] = useState<JobLevel[]>([])

  const list = useListQuery<User>((params) => userApi.list(params), {
    institution: institution || undefined,
    filters: {
      status: status || undefined,
      mfa: mfaFilter ? mfaFilter === 'bound' : undefined,
      department_id: departmentFilter ? Number(departmentFilter) : undefined,
      job_level_id: jobLevelFilter ? Number(jobLevelFilter) : undefined,
    },
  })
  const { pending, run } = useMutation()

  const [removing, setRemoving] = useState<User | null>(null)
  const [disabling, setDisabling] = useState<User | null>(null)

  // 部门下拉数据（筛选用）挂载后加载一次；组织切换会重建组件，不会用到旧组织的部门
  const loadDepartments = useCallback(async () => {
    try {
      const result = await departmentApi.list({ institution, page: 1, size: 200 })
      setDepartments(result.items ?? [])
    } catch {
      setDepartments([])
    }
  }, [institution])
  useOnceKey(`users:departments:${institution}`, loadDepartments)

  const loadJobLevels = useCallback(async () => {
    try {
      const result = await jobLevelApi.list({ institution, page: 1, size: 200 })
      setJobLevels(result.items ?? [])
    } catch {
      setJobLevels([])
    }
  }, [institution])
  useOnceKey(`users:job-levels:${institution}`, loadJobLevels)

  const columns: Column<User>[] = [
    {
      key: 'avatar',
      headerKey: 'users.avatarColumn',
      headClassName: 'w-12',
      render: (user) => <UserAvatar user={user} />,
    },
    {
      key: 'name',
      headerKey: 'users.name',
      render: (user) => (
        // 姓名 + 账号两行整体可点：走 Button 的 link variant，
        // 用 flex-col 覆盖它的 inline-flex 居中布局（不再手写 hover:underline）
        <Button
          type="button"
          variant="link"
          className="h-auto max-w-[16rem] flex-col items-start gap-0 p-0 text-left"
          onClick={() => navigate(`/institutions/${institutionId}/users/${user.id}`)}
        >
          <span className="truncate font-medium">{user.name}</span>
          {/* 副标题统一显示邮箱；没有就留占位，不再退化成 uid（UUID 在列表里既难认又无意义） */}
          <span className="text-muted-foreground truncate text-xs">{user.email || '-'}</span>
        </Button>
      ),
    },
    {
      key: 'display',
      headerKey: 'users.displayName',
      render: (user) => <span className="whitespace-nowrap">{user.display_name || '-'}</span>,
    },
    {
      key: 'job_level',
      headerKey: 'users.jobLevel',
      render: (user) => (user.job_level ? <Badge variant="secondary">{user.job_level}</Badge> : '-'),
    },
    {
      key: 'department',
      headerKey: 'users.department',
      render: (user) => {
        // 多部门挂职：详情与列表同一口径，全部展示（主部门在前）
        const paths = user.department_paths?.length
          ? user.department_paths
          : user.department_path
            ? [user.department_path]
            : []
        if (paths.length === 0) return <span className="text-muted-foreground text-xs">-</span>
        // 部门多时只显示第一个 + "+N"，其余收进 title：
        // 之前全部平铺会把行高撑成好几行（一个用户挂 5 个部门就占 5 行）
        const [primary, ...rest] = paths
        return (
          <div className="flex max-w-[18rem] items-center gap-1">
            <Badge variant="outline" className="min-w-0 text-xs font-normal">
              <span className="truncate" title={primary}>
                {primary}
              </span>
            </Badge>
            {rest.length > 0 ? (
              <Badge variant="secondary" className="shrink-0 text-xs font-normal" title={rest.join('\n')}>
                +{rest.length}
              </Badge>
            ) : null}
          </div>
        )
      },
    },
    {
      key: 'status',
      headerKey: 'users.status',
      render: (user) => (
        <div className="flex flex-wrap items-center gap-1">
          {user.is_admin ? <Badge variant="info">{t('users.isAdmin')}</Badge> : null}
          {user.is_forbidden ? <Badge variant="destructive">{t('users.statusDisabled')}</Badge> : null}
          {user.is_deleted ? <Badge variant="destructive">{t('users.statusDeleted')}</Badge> : null}
          {!user.is_forbidden && !user.is_deleted ? (
            <Badge variant="success">{t('users.statusActive')}</Badge>
          ) : null}
          {user.mfa_enabled ? <Badge variant="secondary">{t('users.mfa')}</Badge> : null}
          {!user.has_password ? <Badge variant="outline">{t('users.noPassword')}</Badge> : null}
        </div>
      ),
    },
    {
      key: 'lastSignin',
      headerKey: 'users.lastSignin',
      render: (user) => (
        <span className="text-muted-foreground text-xs whitespace-nowrap">
          {user.last_signin_time ? formatRelative(user.last_signin_time) : t('users.neverSignin')}
        </span>
      ),
    },
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (user) => (
        <RowActionsMenu
          primary={{
            labelKey: 'users.viewDetail',
            onSelect: () => navigate(`/institutions/${institutionId}/users/${user.id}`),
          }}
          actions={[
            {
              labelKey: user.is_forbidden ? 'users.enable' : 'users.disable',
              onSelect: () => setDisabling(user),
            },
            { labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(user) },
          ]}
        />
      ),
    },
  ]

  return (
    <PageShell
      titleKey="users.title"
      subtitleKey="users.subtitle"
      actions={
        <Button onClick={() => navigate(`/institutions/${institutionId}/users/new`)}>
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(user) => user.id}
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
            placeholder={t('users.searchPlaceholder')}
            filters={[
              {
                label: t('users.filterStatus'),
                value: status,
                options: [
                  { value: 'active', label: t('users.statusActive') },
                  { value: 'disabled', label: t('users.statusDisabled') },
                  { value: 'deleted', label: t('users.statusDeleted') },
                ],
                onChange: setStatus,
              },
              {
                label: t('users.filterDepartment'),
                value: departmentFilter,
                options: departments.map((dept) => ({
                  value: String(dept.id),
                  label: dept.name_path || dept.name,
                })),
                onChange: setDepartmentFilter,
                allLabel: t('users.filterDepartmentAll'),
              },
              {
                label: t('users.filterJobLevel'),
                value: jobLevelFilter,
                options: jobLevels.map((level) => ({ value: String(level.id), label: level.name })),
                onChange: setJobLevelFilter,
                allLabel: t('users.filterJobLevelAll'),
              },
              {
                label: t('users.filterMfa'),
                value: mfaFilter,
                options: [
                  { value: 'bound', label: t('users.filterMfaBound') },
                  { value: 'unbound', label: t('users.filterMfaUnbound') },
                ],
                onChange: setMfaFilter,
              },
            ]}
          />
        }
      />

      <ConfirmDialog
        open={Boolean(disabling)}
        onOpenChange={(open) => !open && setDisabling(null)}
        titleKey={disabling?.is_forbidden ? 'users.enableConfirmTitle' : 'users.disableConfirmTitle'}
        descriptionKey={disabling?.is_forbidden ? 'users.enableConfirmBody' : 'users.disableConfirmBody'}
        descriptionParams={{ name: disabling?.name }}
        loading={pending}
        onConfirm={async () => {
          if (!disabling) return
          const ok = await run(() => userApi.setDisabled(disabling.id, !disabling.is_forbidden))
          if (ok) {
            setDisabling(null)
            await list.reload()
          }
        }}
      />

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="users.deleteConfirmBody"
        descriptionParams={{ name: removing?.name }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(() => userApi.remove(removing.id), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
