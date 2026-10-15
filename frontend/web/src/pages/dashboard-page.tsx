import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import {
  Activity,
  Boxes,
  Building2,
  FileClock,
  KeyRound,
  Layers3,
  Plus,
  ShieldCheck,
  Sparkles,
  Users,
  UsersRound,
  Waypoints,
} from 'lucide-react'
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { SummaryStrip, type SummaryItem } from '@/widgets/layout/summary-strip'
import { formatRelative } from '@/shared/lib/format'
import { formatLongDate, greetingPeriodKey } from '@/shared/lib/long-date'
import type { AuditRecord, InstitutionStats } from '@/shared/api/types'
import {
  applicationApi,
  groupApi,
  institutionApi,
  permissionApi,
  providerApi,
  recordApi,
  roleApi,
  syncerApi,
  userApi,
} from '@/features/iam/api'
import { useAuth } from '@/features/auth/auth-context'
import { useCurrentInstitution } from '@/features/iam/institution-context'

interface Stat {
  key: string
  labelKey: string
  icon: typeof Users
  to: (institution_id: number) => string
  value: number | null
}

/**
 * 平台总览：先说"你是谁、现在是今天什么时候"（问候 + 日期），再说"规模多大"（KPI 卡片行），
 * 最后才是明细（按组织聚合 / 最近操作 / 快速开始）。
 *
 * KPI 卡片是**入口**而不是装饰：每张卡都指向对应资源列表，点进去就是那一页。
 * 卡片上不写"同比 +20%"这类趋势——后端没有历史快照，编一个数字比不写更糟。
 */
export default function DashboardPage() {
  const { t, i18n } = useTranslation()
  const { isAdmin, status } = useAuth()
  const { institution, institutions } = useCurrentInstitution()
  const workspace = institution || ''
  const [stats, setStats] = useState<Stat[]>(() => [
    { key: 'users', labelKey: 'dashboard.users', icon: Users, to: (i: number) => `/institutions/${i}/users`, value: null },
    {
      key: 'applications',
      labelKey: 'dashboard.applications',
      icon: Layers3,
      to: (i: number) => `/institutions/${i}/applications`,
      value: null,
    },
    {
      key: 'providers',
      labelKey: 'dashboard.providers',
      icon: Waypoints,
      to: (i: number) => `/institutions/${i}/providers`,
      value: null,
    },
    // 身份连接：入方向同步与出方向供给共用同一页（历史路径 `/institutions/:id/syncers` 已不存在）
    {
      key: 'syncers',
      labelKey: 'dashboard.syncers',
      icon: Sparkles,
      to: (i: number) => `/institutions/${i}/connections`,
      value: null,
    },
    {
      key: 'groups',
      labelKey: 'dashboard.groups',
      icon: UsersRound,
      to: (i: number) => `/institutions/${i}/groups`,
      value: null,
    },
    { key: 'roles', labelKey: 'dashboard.roles', icon: ShieldCheck, to: () => '/roles', value: null },
    { key: 'permissions', labelKey: 'dashboard.permissions', icon: KeyRound, to: () => '/permissions', value: null },
    { key: 'orgs', labelKey: 'dashboard.orgs', icon: Building2, to: () => '/institutions', value: null },
  ])
  const [records, setRecords] = useState<AuditRecord[]>([])
  const [orgStats, setOrgStats] = useState<InstitutionStats[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      const fetchers: { key: string; run: () => Promise<{ total: number }> }[] = [
        { key: 'users', run: () => userApi.list({ institution, size: 1 }) },
        { key: 'applications', run: () => applicationApi.list({ institution, size: 1 }) },
        { key: 'providers', run: () => providerApi.list({ institution, size: 1 }) },
        { key: 'roles', run: () => roleApi.list({ scope: 'platform', size: 1 }) },
        { key: 'permissions', run: () => permissionApi.list({ scope: 'platform', size: 1 }) },
        { key: 'groups', run: () => groupApi.list({ institution, size: 1 }) },
        { key: 'syncers', run: () => syncerApi.list({ institution, size: 1 }) },
        { key: 'orgs', run: () => institutionApi.list({ size: 1 }) },
      ]

      const results = await Promise.all(
        fetchers.map(async (fetcher) => {
          try {
            const result = await fetcher.run()
            return { key: fetcher.key, total: result.total ?? 0 }
          } catch {
            return { key: fetcher.key, total: null }
          }
        }),
      )
      if (cancelled) return
      setStats((current) =>
        current.map((stat) => {
          const found = results.find((item) => item.key === stat.key)
          return { ...stat, value: found ? found.total : null }
        }),
      )

      try {
        const recent = await recordApi.list({ scope: 'platform', size: 8 })
        if (!cancelled) setRecords(recent.items ?? [])
      } catch {
        if (!cancelled) setRecords([])
      }
      try {
        const aggregated = await institutionApi.statsAll()
        if (!cancelled) setOrgStats(aggregated.items ?? [])
      } catch {
        if (!cancelled) setOrgStats([])
      }
      if (!cancelled) setLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [institution])

  const greetingKey = greetingPeriodKey(new Date().getHours())
  const displayName = status?.display_name || status?.name || ''
  const heading = displayName
    ? t('dashboard.greeting', { period: t(greetingKey), name: displayName })
    : t('dashboard.greetingAnonymous', { period: t(greetingKey) })

  const statItems: SummaryItem[] = stats.map((stat) => ({
    label: t(stat.labelKey),
    icon: stat.icon,
    value: stat.value === null ? <Skeleton className="h-6 w-14" /> : stat.value,
    to: workspace ? stat.to(Number(workspace)) : undefined,
  }))

  /**
   * 组织聚合表的行标识与跳转地址。
   *
   * `/institutions/stats` 下发的是组织**标识**（`built-in`）而不是主键 id，而
   * 「地址栏里的组织段一律是主键 id」（WorkspaceLayout 会把标识就地重定向到 id）。
   * 这里优先用组织列表把标识翻成 id，翻不到（列表未加载 / 已删除）时退回标识，
   * 由 WorkspaceLayout 的兼容重定向兜底——绝不再拼出 `/institutions/undefined`。
   */
  const orgTarget = (item: InstitutionStats) => {
    const matched = institutions.find((org) => org.name === item.institutionID)
    return matched ? `/institutions/${matched.id}` : `/institutions/${item.institutionID}`
  }

  return (
    <PageShell
      title={heading}
      description={formatLongDate(new Date(), i18n.resolvedLanguage ?? 'zh-CN')}
      actions={
        isAdmin && workspace ? (
          <Button asChild>
            <Link to={`/institutions/${workspace}/applications/new`}>
              <Plus className="size-4" />
              {t('applications.createTitle')}
            </Link>
          </Button>
        ) : null
      }
    >
      <SummaryStrip items={statItems} className="sm:grid-cols-4" />

      <Card>
        <CardHeader>
          <CardTitle>{t('dashboard.byInstitution')}</CardTitle>
          <CardDescription>{t('dashboard.byInstitutionHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          {orgStats.length === 0 ? (
            <p className="text-muted-foreground text-[13px]">{t('common.empty')}</p>
          ) : (
            <div className="border-border/70 overflow-hidden rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>{t('auth.institution')}</TableHead>
                    <TableHead>{t('dashboard.users')}</TableHead>
                    <TableHead>{t('dashboard.applications')}</TableHead>
                    <TableHead>{t('dashboard.roles')}</TableHead>
                    <TableHead>{t('dashboard.permissions')}</TableHead>
                    <TableHead>{t('dashboard.total')}</TableHead>
                    <TableHead className="text-right">{t('common.actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {orgStats.map((item) => (
                    <TableRow key={item.institutionID}>
                      <TableCell className="text-foreground font-medium">
                        <Link to={orgTarget(item)} className="hover:text-primary">
                          {item.display_name || item.institutionID}
                        </Link>
                      </TableCell>
                      <TableCell>{item.counts.users ?? 0}</TableCell>
                      <TableCell>{item.counts.applications ?? 0}</TableCell>
                      <TableCell>{item.counts.roles ?? 0}</TableCell>
                      <TableCell>{item.counts.permissions ?? 0}</TableCell>
                      <TableCell className="text-foreground font-medium tabular-nums">{item.total}</TableCell>
                      <TableCell className="text-right">
                        <Button asChild variant="ghost" size="sm">
                          <Link to={orgTarget(item)}>{t('institutions.manage')}</Link>
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-5 lg:grid-cols-[1.35fr_1fr]">
        <Card>
          {/* 注意：CardHeader 基类带 `grid`，这里必须显式给 `flex` 才能覆盖 display，否则按钮会掉到第二行 */}
          <CardHeader className="flex flex-wrap items-center justify-between gap-3">
            <div className="space-y-1.5">
              <CardTitle>{t('dashboard.recentRecords')}</CardTitle>
              <CardDescription>{t('records.subtitle')}</CardDescription>
            </div>
            <Button asChild variant="ghost" size="sm">
              <Link to="/settings/audit">{t('nav.audit')} →</Link>
            </Button>
          </CardHeader>
          <CardContent>
            {loading ? (
              <div className="space-y-2">
                {Array.from({ length: 5 }).map((_, index) => (
                  <Skeleton key={index} className="h-11 w-full" />
                ))}
              </div>
            ) : records.length === 0 ? (
              <div className="text-muted-foreground flex items-center gap-2 py-6 text-[13px]">
                <FileClock className="size-4" />
                {t('common.empty')}
              </div>
            ) : (
              <ul className="divide-border/60 divide-y">
                {records.map((record) => (
                  <li key={record.id} className="flex items-center gap-3 py-2.5">
                    <span className="bg-primary/10 text-primary grid size-8 shrink-0 place-items-center rounded-[10px]">
                      <Activity className="size-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-[13px] font-medium">{record.action || '-'}</p>
                      <p className="text-muted-foreground truncate text-[11px]">
                        {record.user || '-'}
                        {record.institution_id ? ` · ${record.institution_id}` : ''}
                      </p>
                    </div>
                    <span className="text-muted-foreground shrink-0 text-[11px]">
                      {formatRelative(record.created_at)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Boxes className="size-4" /> {t('dashboard.quickStart')}
            </CardTitle>
            <CardDescription>{t('dashboard.quickStartHint')}</CardDescription>
          </CardHeader>
          <CardContent>
            <ol className="space-y-3 text-[13px]">
              {['dashboard.step1', 'dashboard.step2', 'dashboard.step3', 'dashboard.step4'].map((key, index) => (
                <li key={key} className="flex items-start gap-3">
                  <span className="bg-primary/10 text-primary mt-0.5 grid size-5 shrink-0 place-items-center rounded-full text-[11px] font-semibold">
                    {index + 1}
                  </span>
                  <span className="text-muted-foreground leading-relaxed">{t(key)}</span>
                </li>
              ))}
            </ol>
          </CardContent>
        </Card>
      </div>
    </PageShell>
  )
}
