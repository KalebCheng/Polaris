import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, ShieldCheck, ShieldOff } from 'lucide-react'
import { Avatar, AvatarFallback, AvatarImage, Badge, Button, Skeleton, Tabs, TabsList, TabsTrigger } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ApiError } from '@/shared/api/client'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { LoadError } from '@/shared/components/load-error'
import { describeError } from '@/shared/lib/errors'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import type { User } from '@/shared/api/types'
import { userApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useMutation } from '../hooks/use-mutation'
import { OverviewTab } from '../components/user/overview-tab'
import { SecurityTab } from '../components/user/security-tab'
import { AuthorizationTab } from '../components/user/authorization-tab'
import { RelationsTab } from '../components/user/relations-tab'
import { ActivityTab } from '../components/user/activity-tab'

type TabKey = 'overview' | 'security' | 'authorization' | 'relations' | 'activity'

const TABS: { key: TabKey; labelKey: string }[] = [
  { key: 'overview', labelKey: 'users.tabOverview' },
  { key: 'security', labelKey: 'users.tabSecurity' },
  { key: 'authorization', labelKey: 'users.tabAuthorization' },
  { key: 'relations', labelKey: 'users.tabRelations' },
  { key: 'activity', labelKey: 'users.tabActivity' },
]

/**
 * 用户详情页（/institutions/:inst/users/:id）。
 *
 * 信息架构：概览 / 安全与认证 / 授权 / 关联 / 活动 五个标签，**首次激活才请求**该标签的数据
 * （用 `visited` 记录，避免"打开页面就把五份数据全拉一遍"；StrictMode 下的重复请求由 ref 守卫挡掉）。
 */
export default function UserDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const institution = useWorkspaceInstitution()
  const institutionId = useWorkspaceInstitutionId()
  const { pending, run } = useMutation()

  const [user, setUser] = useState<User | null>(null)
  const [tab, setTab] = useState<TabKey>('overview')
  const [visited, setVisited] = useState<Set<TabKey>>(() => new Set<TabKey>(['overview']))
  const [loading, setLoading] = useState(true)
  // 与"用户不存在"区分开：其他错误（网络/500/接口不存在）必须显式报错，不能一律说成"不存在"
  const [loadError, setLoadError] = useState('')
  const [disableOpen, setDisableOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  // 引导请求按 key 守卫：同一 id 只发一次（开发环境 StrictMode 会双调用 effect）
  const bootstrapped = useRef<string>('')

  const load = useCallback(
    async (force = false) => {
      if (!id) return
      if (!force && bootstrapped.current === id) return
      bootstrapped.current = id
      setLoading(true)
      setLoadError('')
      try {
        const detail = await userApi.get(Number(id), institution || undefined)
        setUser(detail)
      } catch (cause) {
        if (cause instanceof ApiError && cause.status === 404) {
          setUser(null)
        } else {
          // 保留已加载的内容，只在顶部提示"没读到最新数据"
          setLoadError(describeError(cause, t))
        }
      } finally {
        setLoading(false)
      }
    },
    [id, institution, t],
  )

  useEffect(() => {
    void load()
  }, [load])

  const reload = useCallback(() => load(true), [load])

  const selectTab = (next: string) => {
    const key = next as TabKey
    setTab(key)
    setVisited((prev) => (prev.has(key) ? prev : new Set(prev).add(key)))
  }

  if (loading && !user) {
    return (
      <PageShell title={t('users.detailTitle')}>
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </PageShell>
    )
  }

  if (!user) {
    return (
      <PageShell title={t('users.detailTitle')}>
        <div className="space-y-3">
          {loadError ? (
            <LoadError onRetry={() => void reload()} detail={loadError} />
          ) : (
            <p className="text-muted-foreground text-sm">{t('users.notFound')}</p>
          )}
          <Button variant="outline" onClick={() => navigate(`/institutions/${institutionId}/users`)}>
            <ArrowLeft className="size-4" /> {t('users.backToList')}
          </Button>
        </div>
      </PageShell>
    )
  }

  const initials = (user.display_name || user.name).slice(0, 1).toUpperCase()

  return (
    <PageShell
      title={
        <span className="flex items-center gap-3">
          <Avatar className="size-10">
            {user.avatar ? <AvatarImage src={user.avatar} alt={user.name} /> : null}
            <AvatarFallback>{initials}</AvatarFallback>
          </Avatar>
          <span className="flex min-w-0 flex-col">
            <span className="truncate text-lg font-semibold">{user.display_name || user.name}</span>
            <span className="text-muted-foreground truncate text-xs font-normal">
              {user.name}
              {user.email ? ` · ${user.email}` : ''}
            </span>
          </span>
        </span>
      }
      actions={
        <div className="flex items-center gap-2">
          <Link to={`/institutions/${institutionId}/users`} className="text-muted-foreground text-sm hover:underline">
            {t('users.backToList')}
          </Link>
          <RowActionsMenu
            primary={{
              labelKey: 'users.edit',
              onSelect: () => navigate(`/institutions/${institutionId}/users/${user.id}/edit`),
            }}
            actions={[
              {
                labelKey: user.is_forbidden ? 'users.enable' : 'users.disable',
                onSelect: () => setDisableOpen(true),
              },
              {
                labelKey: user.is_admin ? 'users.unsetAdmin' : 'users.setAdmin',
                onSelect: () => {
                  void run(async () => {
                    await userApi.setAdmin(user.id, !user.is_admin)
                    await reload()
                  })
                },
              },
              { labelKey: 'common.delete', destructive: true, onSelect: () => setDeleteOpen(true) },
            ]}
          />
        </div>
      }
    >
      {loadError ? (
        <div className="pb-3">
          <LoadError compact onRetry={() => void reload()} detail={loadError} />
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-1.5 pb-3">
        {user.is_admin ? <Badge variant="info">{t('users.isAdmin')}</Badge> : null}
        {user.is_forbidden ? <Badge variant="destructive">{t('users.isForbidden')}</Badge> : null}
        {user.is_deleted ? <Badge variant="destructive">{t('users.isDeleted')}</Badge> : null}
        {user.is_online ? <Badge variant="success">{t('users.isOnline')}</Badge> : null}
        {user.department_path ? <Badge variant="outline">{user.department_path}</Badge> : null}
        <Badge variant={user.mfa_enabled ? 'secondary' : 'outline'}>
          {user.mfa_enabled ? <ShieldCheck className="size-3" /> : <ShieldOff className="size-3" />}
          {user.mfa_enabled ? t('users.mfa') : t('users.mfaNotBound')}
        </Badge>
        {!user.has_password ? <Badge variant="outline">{t('users.noPassword')}</Badge> : null}
      </div>

      <Tabs value={tab} onValueChange={selectTab}>
        <TabsList>
          {TABS.map((item) => (
            <TabsTrigger key={item.key} value={item.key}>
              {t(item.labelKey)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <div className="pt-4">
        {visited.has('overview') ? (
          <div hidden={tab !== 'overview'}>
            <OverviewTab user={user} />
          </div>
        ) : null}
        {visited.has('security') ? (
          <div hidden={tab !== 'security'}>
            <SecurityTab user={user} onChanged={reload} />
          </div>
        ) : null}
        {visited.has('authorization') ? (
          <div hidden={tab !== 'authorization'}>
            <AuthorizationTab user={user} onChanged={reload} />
          </div>
        ) : null}
        {visited.has('relations') ? (
          <div hidden={tab !== 'relations'}>
            <RelationsTab user={user} onChanged={reload} />
          </div>
        ) : null}
        {visited.has('activity') ? (
          <div hidden={tab !== 'activity'}>
            <ActivityTab user={user} />
          </div>
        ) : null}
      </div>

      <ConfirmDialog
        open={disableOpen}
        onOpenChange={setDisableOpen}
        titleKey={user.is_forbidden ? 'users.enableConfirmTitle' : 'users.disableConfirmTitle'}
        descriptionKey={user.is_forbidden ? 'users.enableConfirmBody' : 'users.disableConfirmBody'}
        descriptionParams={{ name: user.name }}
        loading={pending}
        onConfirm={async () => {
          const ok = await run(() => userApi.setDisabled(user.id, !user.is_forbidden))
          if (ok) {
            setDisableOpen(false)
            await reload()
          }
        }}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="users.deleteConfirmBody"
        descriptionParams={{ name: user.name }}
        loading={pending}
        onConfirm={async () => {
          const ok = await run(async () => {
            await userApi.remove(user.id)
            navigate(`/institutions/${institutionId}/users`)
          }, 'common.deleted')
          if (!ok) setDeleteOpen(false)
        }}
      />
    </PageShell>
  )
}
