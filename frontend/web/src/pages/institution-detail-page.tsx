import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowLeft, ExternalLink, Pencil } from 'lucide-react'
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
import { institutionApi, jobLevelApi } from '@/features/iam/api'
import { useCurrentInstitution } from '@/features/iam/institution-context'
import { useWorkspaceInstitution } from '@/features/iam/institution-context'
import { PASSWORD_OPTIONS } from '@/features/iam/lib/password-policy'
import type { Institution, InstitutionStats } from '@/shared/api/types'

/** 资源维度 → 对应管理页（点击后进入该组织的资源管理） */
const resourceLinks: { key: string; labelKey: string; to: string }[] = [
  { key: 'users', labelKey: 'nav.users', to: 'users' },
  { key: 'jobTitles', labelKey: 'nav.jobTitles', to: 'jobTitles' },
  { key: 'applications', labelKey: 'nav.applications', to: 'applications' },
  { key: 'providers', labelKey: 'nav.providers', to: 'providers' },
  { key: 'roles', labelKey: 'nav.roles', to: 'roles' },
  { key: 'permissions', labelKey: 'nav.permissions', to: 'permissions' },
  { key: 'connections', labelKey: 'nav.connections', to: 'connections' },
  { key: 'tokens', labelKey: 'nav.tokens', to: 'tokens' },
]

/** 只读信息项（标签 + 值） */
function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{value}</dd>
    </div>
  )
}

/**
 * 组织概览（工作区入口）：以组织为中心的聚合视图。
 * 组织配置的**编辑**收敛到独立页面 `/institutions/:name/edit`（此处只读展示 + 跳转），
 * 避免同一种编辑在两个地方各写一遍导致漂移。
 */
export default function InstitutionDetailPage() {
  const { t } = useTranslation()
  // 组织来自 URL（/institutions/:inst），与工作区上下文同源，避免参数名不一致导致取空
  const name = useWorkspaceInstitution()
  // 平台级组织接口按 id 访问；URL 里的仍是组织名（租户 slug），故这里做一次映射
  const { institutions } = useCurrentInstitution()
  const orgID = institutions.find((item) => item.name === name)?.id ?? 0
  const navigate = useNavigate()

  const [org, setOrg] = useState<Institution | null>(null)
  const [stats, setStats] = useState<InstitutionStats | null>(null)
  const [levelDistribution, setLevelDistribution] = useState<{ name: string; count: number }[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // orgID 由组织列表映射而来（列表未就绪时先等待，避免按 0 请求）
    if (!name || orgID <= 0) return
    let cancelled = false
    setLoading(true)
    void (async () => {
      try {
        const [detail, aggregated, distribution] = await Promise.all([
          institutionApi.get(orgID),
          institutionApi.stats(orgID),
          jobLevelApi.distribution(String(orgID)).catch(() => ({ items: [] })),
        ])
        if (cancelled) return
        setOrg(detail)
        setStats(aggregated)
        setLevelDistribution(distribution.items ?? [])
      } catch {
        if (!cancelled) setOrg(null)
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [name, orgID])

  const optionLabels = (options: string[] | undefined) =>
    (options ?? [])
      .map((value) => {
        const option = PASSWORD_OPTIONS.find((item) => item.value === value)
        return option ? t(option.labelKey) : value
      })
      .join('、')

  return (
    <PageShell
      titleKey="institutions.overviewTitle"
      subtitleKey="institutions.detailSubtitle"
      actions={
        <>
          <Button variant="outline" onClick={() => navigate('/institutions')}>
            <ArrowLeft className="size-4" /> {t('institutions.allInstitutions')}
          </Button>
          <Button disabled={!org} onClick={() => navigate(`/institutions/${orgID}/edit`)}>
            <Pencil className="size-4" /> {t('institutions.editOrg')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 8 }).map((_, index) => (
            <Skeleton key={index} className="h-24 w-full" />
          ))}
        </div>
      ) : !org ? (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            {t('institutions.notFound')}
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">{org.name}</Badge>
            {org.disable_signin ? (
              <Badge variant="destructive">{t('institutions.disableSignin')}</Badge>
            ) : null}
            {org.use_email_as_username ? (
              <Badge variant="outline">{t('institutions.useEmailAsUsername')}</Badge>
            ) : null}
            <span className="text-muted-foreground text-xs">
              {t('institutions.totalResources')}: {stats?.total ?? 0}
            </span>
            {stats?.last_activity ? (
              <span className="text-muted-foreground text-xs">
                {t('institutions.lastActivity')}: {stats.last_activity}
              </span>
            ) : null}
          </div>

          {/* 资源聚合：点击进入该组织的对应资源管理 */}
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {resourceLinks.map((item) => (
              <Card key={item.key} className="gap-3 py-4">
                <CardContent className="space-y-2">
                  <p className="text-muted-foreground text-xs">{t(item.labelKey)}</p>
                  <p className="text-2xl font-semibold tracking-tight">{stats?.counts?.[item.key] ?? 0}</p>
                  <Button asChild variant="ghost" size="sm" className="h-6 px-0 text-xs">
                    <Link to={`/institutions/${name}/${item.to}`}>
                      {t('institutions.manage')} <ExternalLink className="ml-1 size-3" />
                    </Link>
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t('institutions.jobLevelDistribution')}</CardTitle>
              <CardDescription>{t('institutions.jobLevelDistributionHint')}</CardDescription>
            </CardHeader>
            <CardContent>
              {levelDistribution.length === 0 ? (
                <p className="text-muted-foreground text-sm">{t('institutions.jobLevelDistributionEmpty')}</p>
              ) : (
                <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  {levelDistribution.map((item) => (
                    <li
                      key={item.name}
                      className="bg-muted/40 flex items-center justify-between rounded-lg px-3 py-2 text-sm"
                    >
                      <span className="truncate">{item.name}</span>
                      <span className="font-medium tabular-nums">{item.count}</span>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t('institutions.settings')}</CardTitle>
              <CardDescription>{t('institutions.subtitle')}</CardDescription>
            </CardHeader>
            <CardContent>
              <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
                <InfoRow
                  label={t('institutions.identifier')}
                  value={<span className="font-mono text-xs">{org.name}</span>}
                />
                <InfoRow label={t('institutions.name')} value={org.display_name || '—'} />
                <InfoRow label={t('institutions.website')} value={org.website_url || '—'} />
                <InfoRow label={t('institutions.passwordType')} value={org.password_type || '—'} />
                <InfoRow
                  label={t('institutions.passwordOptions')}
                  value={optionLabels(org.password_options) || '—'}
                />
                <InfoRow
                  label={t('institutions.passwordExpireDays')}
                  value={org.password_expire_days || '—'}
                />
                <InfoRow label={t('institutions.description')} value={org.description || '—'} />
              </dl>
            </CardContent>
          </Card>
        </>
      )}
    </PageShell>
  )
}
