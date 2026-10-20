import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Button, Skeleton, Tabs, TabsList, TabsTrigger } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { formatRelative } from '@/shared/lib/format'
import type { ConnectionDirection, Connector, Syncer } from '@/shared/api/types'
import { connectorApi, syncerApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { useAuth } from '@/shared/auth/auth-context'
import { toast } from '@/shared/ui/toast'
import { describeError } from '@/shared/lib/errors'
import { ConnectionCard, type ConnectionCardView } from '../components/connection-card'
import { ConnectionRuns } from '../components/connection-runs'
import { ConnectorRuns } from '../components/connector-runs'
import { ConnectorReconcile } from '../components/connector-reconcile'

/** 入方向类型支持的图标规格（与后端 catalog 的 icon 声明一致） */
const IN_ICONS: Record<string, { key: string; by_sub_type?: Record<string, string> }> = {
  dingtalk: { key: 'dingtalk' },
  wecom: { key: 'wecom' },
  lark: { key: 'feishu', by_sub_type: { lark: 'lark' } },
  ldap: { key: 'ldap' },
  database: { key: 'database', by_sub_type: { mysql: 'mysql', postgres: 'postgres' } },
}

/**
 * 身份连接（入方向 / 出方向）。
 * 路由：/institutions/:inst/connections?direction=in|out
 *
 * 形态对齐成熟产品：方向标签 + 类型卡片目录（添加）+ 实例卡片（状态 / 检查 / 修改）。
 * 新增与编辑一律走**整页向导**（`/connections/new` 与 `/connections/:id`），不使用弹窗。
 */
export default function ConnectionsPage() {
  const { t } = useTranslation()
  const { can } = useAuth()
  const institution = useWorkspaceInstitution()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const direction: ConnectionDirection = params.get('direction') === 'out' ? 'out' : 'in'

  const [removing, setRemoving] = useState<{ id: number; name: string } | null>(null)
  const [logs, setLogs] = useState<{ id: number; name: string } | null>(null)
  const [provisionRuns, setProvisionRuns] = useState<{ connector: string; name: string } | null>(null)
  const [reconcile, setReconcile] = useState<{ id: number; applicationID: number; name: string } | null>(null)
  const { pending, run } = useMutation()

  const syncers = useListQuery<Syncer>((query) => syncerApi.list(query), {
    institution: institution || undefined,
  })
  const connectors = useListQuery<Connector>((query) => connectorApi.list(query), {
    institution: institution || undefined,
  })

  /** 新增一律走**整页**：先在选择页挑接入方式，再进入分步向导（不使用弹窗，也不在列表内联展开） */
  const openNew = () => navigate(`/institutions/${institution}/connections/new?direction=${direction}`)

  const loading = direction === 'in' ? syncers.loading : connectors.loading
  const reload = direction === 'in' ? syncers.reload : connectors.reload

  const cards: ConnectionCardView[] =
    direction === 'in'
      ? syncers.items.map((syncer) => ({
          name: syncer.display_name || syncer.name,
          subtitle: syncer.name,
          icon: IN_ICONS[syncer.type] ?? { key: syncer.type },
          type: syncer.type,
          subType: syncer.sub_type,
          tags: [t('connections.directionIn'), syncer.type, syncer.sub_type || ''].filter(Boolean),
          meta: [
            t('connections.lastRunLabel', { value: formatRelative(syncer.last_run_time) }),
            syncer.last_status || t('connections.never'),
          ],
          status: {
            tone: !syncer.is_enabled ? 'neutral' : syncer.last_status === 'failed' ? 'danger' : 'success',
            label: !syncer.is_enabled
              ? t('connections.statusDisabled')
              : syncer.last_status === 'failed'
                ? t('connections.statusFailed')
                : t('connections.statusReady'),
          },
          sections: [
            {
              label: t('connections.pullTitle'),
              hint: [
                syncer.sync_users ? t('connections.syncUsers') : '',
                syncer.sync_groups ? t('connections.syncGroups') : '',
              ]
                .filter(Boolean)
                .join(' · '),
              links: [{ label: t('connections.editConfig'), onClick: () => openEdit(syncer.id) }],
            },
            {
              label: t('connections.scheduleTitle'),
              hint: scheduleHint(syncer, t),
              links: [
                { label: t('connections.runNow'), onClick: () => openRun(syncer.id) },
                { label: t('connections.dryRun'), onClick: () => openDryRun(syncer.id) },
                {
                  label: t('connections.viewLogs'),
                  onClick: () => setLogs({ id: syncer.id, name: syncer.display_name || syncer.name }),
                },
              ],
            },
          ],
          onEdit: () => openEdit(syncer.id),
          onDelete: () => setRemoving({ id: syncer.id, name: syncer.name }),
          onToggle: () => void toggleEnabled(syncer),
          toggleLabelKey: syncer.is_enabled ? 'connections.disable' : 'connections.enable',
        }))
      : connectors.items.map((connector) => ({
          name: connector.display_name || connector.name,
          subtitle: connector.name,
          icon: { key: connector.type || 'http' },
          type: connector.type || 'http',
          tags: [t('connections.directionOut'), connector.application || ''].filter(Boolean),
          meta: [
            t('connections.lastRunLabel', { value: formatRelative(connector.last_run_time) }),
            connector.last_status || t('connections.never'),
          ],
          status: {
            tone:
              connector.state !== 'active'
                ? 'neutral'
                : connector.last_status === 'failed'
                  ? 'danger'
                  : 'success',
            label:
              connector.state !== 'active'
                ? t('connections.statusDisabled')
                : connector.last_status === 'failed'
                  ? t('connections.statusFailed')
                  : t('connections.statusReady'),
          },
          sections: [
            {
              label: t('connections.pushTitle'),
              hint: connector.application
                ? t('connections.pushHint', { application: connector.application })
                : t('connections.pushNoApplication'),
              links: [{ label: t('connections.editConfig'), onClick: () => openEdit(connector.id) }],
            },
            {
              label: t('connections.checkTitle'),
              hint: connector.last_message || t('connections.checkHint'),
              links: [
                { label: t('connections.checkNow'), onClick: () => openTest(connector.id) },
                {
                  label: t('connections.reconcile'),
                  onClick: () =>
                    setReconcile({
                      id: connector.id,
                      applicationID: connector.application_id ?? 0,
                      name: connector.display_name || connector.name,
                    }),
                },
                ...(can('provision_history', 'read')
                  ? [
                      {
                        label: t('connections.viewProvisionRecords'),
                        onClick: () =>
                          setProvisionRuns({
                            connector: connector.name,
                            name: connector.display_name || connector.name,
                          }),
                      },
                    ]
                  : []),
              ],
            },
          ],
          onEdit: () => openEdit(connector.id),
          onDelete: () => setRemoving({ id: connector.id, name: connector.name }),
        }))

  /** 启停连接：专用端点只改启用态，计划任务随之启停（不走 PUT，避免冲掉计划配置）。 */
  async function toggleEnabled(syncer: Syncer) {
    try {
      await syncerApi.setEnabled(syncer.id, !syncer.is_enabled)
      toast.success(t('common.saved'))
      await syncers.reload()
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  function openEdit(id: number) {
    navigate(`/institutions/${institution}/connections/${id}?direction=${direction}`)
  }

  /**
   * 试运行：拉取 + 差异 + 同步保护校验，不写任何数据。
   * 不用 useMutation（它只回传成功与否），因为要展示差异数字。
   */
  async function openDryRun(id: number) {
    try {
      const data = await syncerApi.dryRun(id)
      if (data.guard_tripped) {
        toast.error(t('connections.dryRunGuarded', { count: String(data.user_disable) }))
        return
      }
      toast.success(
        t('connections.dryRunResult', {
          create: String(data.user_create),
          update: String(data.user_update),
          disable: String(data.user_disable),
          groups: String(data.dept_create + data.dept_update),
        }),
      )
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  /**
   * 触发同步：入队即返回，然后轮询列表直到该数据源出现结果（最多约 1 分钟）。
   * 界面因此能真实反映「入队中 → 运行中 → 成功/失败」，而不是假装瞬间完成。
   */
  async function openRun(id: number) {
    try {
      const result = await syncerApi.run(id)
      toast.success(t(result.status === 'queued' ? 'connections.runQueued' : 'connections.runAlreadyRunning'))
    } catch (error) {
      toast.error(describeError(error, t))
      return
    }
    await reload()
    if (direction !== 'in') return
    let ticks = 0
    const timer = window.setInterval(async () => {
      ticks += 1
      await reload()
      const target = syncers.items.find((item) => item.id === id)
      const finished = Boolean(target) && target?.last_status !== 'running' && ticks > 1
      if (finished || ticks >= 24) window.clearInterval(timer)
    }, 2500)
  }

  async function openTest(id: number) {
    await run(() => connectorApi.test(id), 'connections.checkOk')
    await reload()
  }

  return (
    <PageShell
      titleKey="connections.title"
      subtitleKey="connections.subtitle"
      actions={
        <Button onClick={openNew}>
          <Plus className="size-4" /> {t('connections.add')}
        </Button>
      }
    >
      <div className="space-y-5">
        <Tabs value={direction} onValueChange={(value) => setParams({ direction: value })}>
          <TabsList>
            <TabsTrigger value="in">{t('connections.directionIn')}</TabsTrigger>
            <TabsTrigger value="out">{t('connections.directionOut')}</TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="space-y-3">
          {loading ? (
            <>
              <Skeleton className="h-32 w-full" />
              <Skeleton className="h-32 w-full" />
            </>
          ) : cards.length ? (
            cards.map((view) => <ConnectionCard key={view.subtitle} view={view} />)
          ) : (
            <div className="surface rounded-2xl p-8 text-center">
              <p className="text-sm font-medium">{t('connections.empty')}</p>
              <p className="text-muted-foreground mt-1 text-xs">{t('connections.emptyHint')}</p>
              <Button className="mt-4" variant="outline" onClick={openNew}>
                <Plus className="size-4" /> {t('connections.add')}
              </Button>
            </div>
          )}
        </div>
      </div>

      <ConnectorReconcile
        open={Boolean(reconcile)}
        onOpenChange={(open) => !open && setReconcile(null)}
        connectorID={reconcile?.id ?? 0}
        applicationID={reconcile?.applicationID ?? 0}
        name={reconcile?.name ?? ''}
      />

      <ConnectionRuns
        open={Boolean(logs)}
        onOpenChange={(open) => !open && setLogs(null)}
        syncerID={logs?.id ?? 0}
        name={logs?.name ?? ''}
      />

      <ConnectorRuns
        open={Boolean(provisionRuns)}
        onOpenChange={(open) => !open && setProvisionRuns(null)}
        institution={institution}
        connector={provisionRuns?.connector ?? ''}
        name={provisionRuns?.name ?? ''}
      />

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: removing?.name }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const target = removing
          const ok = await run(
            () => (direction === 'in' ? syncerApi.remove(target.id) : connectorApi.remove(target.id)),
            'common.deleted',
          )
          if (ok) {
            setRemoving(null)
            await reload()
          }
        }}
      />
    </PageShell>
  )
}

/** 把连接的同步计划翻译成列表页提示文案（唯一来源是 sync_config.schedule）。 */
function scheduleHint(syncer: Syncer, t: TFunction): string {
  const schedule = syncer.sync_config?.schedule
  if (!schedule || !schedule.mode) return t('connections.scheduleNone')
  switch (schedule.mode) {
    case 'interval':
      return t('connections.scheduleEvery', { seconds: schedule.interval_seconds ?? 0 })
    case 'daily':
      return t('connections.scheduleDaily', { times: (schedule.times ?? []).join(', ') })
    case 'cron':
      return t('connections.scheduleCron', { expr: schedule.cron ?? '' })
    default:
      return t('connections.scheduleNone')
  }
}
