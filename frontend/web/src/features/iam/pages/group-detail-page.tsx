import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, Save } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Skeleton,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import type { GroupDetail } from '@/shared/api/types'
import { groupApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useMutation } from '../hooks/use-mutation'
import { UserPicker } from '../components/user-picker'

/**
 * 用户组详情页（/institutions/:inst/groups/:id）。
 *
 * 补齐第三/四阶段的遗留：组是"批量授权的基本单元"，但界面上只有一个逗号分隔的成员串，
 * 既看不到成员明细，也不知道这个组到底把哪些权限授予了组内的人。
 */
export default function GroupDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const institution = useWorkspaceInstitution()
  const institutionId = useWorkspaceInstitutionId()
  const { pending, run } = useMutation()

  const [detail, setDetail] = useState<GroupDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const [members, setMembers] = useState<string[]>([])
  const bootstrapped = useRef('')

  const load = useCallback(
    async (force = false) => {
      if (!id) return
      if (!force && bootstrapped.current === id) return
      bootstrapped.current = id
      setLoading(true)
      try {
        const result = await groupApi.detail(Number(id), institution || undefined)
        setDetail(result)
        setMembers(result.group.users ?? [])
        setNotFound(false)
      } catch {
        setDetail(null)
        setNotFound(true)
      } finally {
        setLoading(false)
      }
    },
    [id, institution],
  )

  useEffect(() => {
    void load()
  }, [load])

  const reload = useCallback(() => load(true), [load])

  if (loading && !detail) {
    return (
      <PageShell title={t('groups.detailTitle')}>
        <Skeleton className="h-64 w-full" />
      </PageShell>
    )
  }

  if (notFound || !detail) {
    return (
      <PageShell title={t('groups.detailTitle')}>
        <div className="space-y-3">
          <p className="text-muted-foreground text-sm">{t('groups.notFound')}</p>
          <Button variant="outline" asChild>
            <Link to={`/institutions/${institutionId}/groups`}>
              <ArrowLeft className="size-4" /> {t('groups.backToList')}
            </Link>
          </Button>
        </div>
      </PageShell>
    )
  }

  const { group, roles, permissions } = detail
  const membersDirty = JSON.stringify([...members].sort()) !== JSON.stringify([...(group.users ?? [])].sort())

  return (
    <PageShell
      title={group.display_name || group.name}
      description={group.manager ? `${group.name} · ${group.manager}` : group.name}
      actions={
        <Link to={`/institutions/${institutionId}/groups`} className="text-muted-foreground text-sm hover:underline">
          {t('groups.backToList')}
        </Link>
      }
    >
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">{t('groups.membersTitle')}</CardTitle>
            <CardDescription>{t('groups.membersHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <UserPicker value={members} onChange={setMembers} institution={institution || undefined} />
            <Button
              size="sm"
              disabled={pending || !membersDirty}
              onClick={async () => {
                const ok = await run(async () => {
                  await groupApi.setMembers(group.id, members)
                  await reload()
                })
                if (!ok) setMembers(group.users ?? [])
              }}
            >
              <Save className="size-4" /> {t('common.save')}
            </Button>
            <p className="text-muted-foreground text-xs">{t('groups.membersEffectHint')}</p>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t('groups.rolesTitle')}</CardTitle>
              <CardDescription>{t('groups.rolesHint')}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-1.5">
                {roles.length === 0 ? (
                  <span className="text-muted-foreground text-sm">{t('groups.rolesEmpty')}</span>
                ) : (
                  roles.map((role) => (
                    <Badge key={role} variant="secondary">
                      {role}
                    </Badge>
                  ))
                )}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t('groups.permissionsTitle')}</CardTitle>
              <CardDescription>{t('groups.permissionsHint')}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-1.5">
                {permissions.length === 0 ? (
                  <span className="text-muted-foreground text-sm">{t('groups.permissionsEmpty')}</span>
                ) : (
                  permissions.map((permission) => (
                    <Badge key={permission.name} variant="outline">
                      {permission.name}
                    </Badge>
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </PageShell>
  )
}
