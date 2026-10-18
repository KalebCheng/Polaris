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
import type { JobTitleDetail } from '@/shared/api/types'
import { jobTitleApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useMutation } from '../hooks/use-mutation'
import { UserPicker } from '../components/user-picker'

/**
 * 职务详情页（/institutions/:inst/job-titles/:id）。
 *
 * 补齐第三/四阶段的遗留：组是"批量授权的基本单元"，但界面上只有一个逗号分隔的成员串，
 * 既看不到成员明细，也不知道这个组到底把哪些权限授予了组内的人。
 */
export default function JobTitleDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const institution = useWorkspaceInstitution()
  const institutionId = useWorkspaceInstitutionId()
  const { pending, run } = useMutation()

  const [detail, setDetail] = useState<JobTitleDetail | null>(null)
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
        const result = await jobTitleApi.detail(Number(id), institution || undefined)
        setDetail(result)
        setMembers(result.jobTitle.users ?? [])
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
      <PageShell title={t('jobTitles.detailTitle')}>
        <Skeleton className="h-64 w-full" />
      </PageShell>
    )
  }

  if (notFound || !detail) {
    return (
      <PageShell title={t('jobTitles.detailTitle')}>
        <div className="space-y-3">
          <p className="text-muted-foreground text-sm">{t('jobTitles.notFound')}</p>
          <Button variant="outline" asChild>
            <Link to={`/institutions/${institutionId}/job-titles`}>
              <ArrowLeft className="size-4" /> {t('jobTitles.backToList')}
            </Link>
          </Button>
        </div>
      </PageShell>
    )
  }

  const { jobTitle, roles, permissions } = detail
  const membersDirty = JSON.stringify([...members].sort()) !== JSON.stringify([...(jobTitle.users ?? [])].sort())

  return (
    <PageShell
      title={jobTitle.display_name || jobTitle.name}
      description={jobTitle.manager ? `${jobTitle.name} · ${jobTitle.manager}` : jobTitle.name}
      actions={
        <Link to={`/institutions/${institutionId}/job-titles`} className="text-muted-foreground text-sm hover:underline">
          {t('jobTitles.backToList')}
        </Link>
      }
    >
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">{t('jobTitles.membersTitle')}</CardTitle>
            <CardDescription>{t('jobTitles.membersHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <UserPicker value={members} onChange={setMembers} institution={institution || undefined} />
            <Button
              size="sm"
              disabled={pending || !membersDirty}
              onClick={async () => {
                const ok = await run(async () => {
                  await jobTitleApi.setMembers(jobTitle.id, members)
                  await reload()
                })
                if (!ok) setMembers(jobTitle.users ?? [])
              }}
            >
              <Save className="size-4" /> {t('common.save')}
            </Button>
            <p className="text-muted-foreground text-xs">{t('jobTitles.membersEffectHint')}</p>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t('jobTitles.rolesTitle')}</CardTitle>
              <CardDescription>{t('jobTitles.rolesHint')}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-1.5">
                {roles.length === 0 ? (
                  <span className="text-muted-foreground text-sm">{t('jobTitles.rolesEmpty')}</span>
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
              <CardTitle className="text-base">{t('jobTitles.permissionsTitle')}</CardTitle>
              <CardDescription>{t('jobTitles.permissionsHint')}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-1.5">
                {permissions.length === 0 ? (
                  <span className="text-muted-foreground text-sm">{t('jobTitles.permissionsEmpty')}</span>
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
