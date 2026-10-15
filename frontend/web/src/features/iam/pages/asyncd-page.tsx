import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { RefreshCw, RotateCcw } from 'lucide-react'
import { Badge, Button, Skeleton, Switch, Tabs, TabsList, TabsTrigger } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { formatDateTime, formatRelative } from '@/shared/lib/format'
import { describeError } from '@/shared/lib/errors'
import { asyncdApi, type AsyncdSchedule, type AsyncdTask } from '../api'

/** 状态标签 → 文案/语气 */
const STATUS_KEYS: Record<string, string> = {
  pending: 'asyncd.statusPending',
  active: 'asyncd.statusActive',
  completed: 'asyncd.statusCompleted',
  archived: 'asyncd.statusArchived',
}

/**
 * asyncd 任务中心（平台级）：异步任务与定时调度的可见性与人工干预。
 *
 * 为什么需要这个页面：队列类问题的通用解法是**可观测 + 可干预**——
 * 只看日志无法回答"有没有堆积、为什么失败、要不要重试"，而"慢操作"恰恰最需要这些。
 */
export default function AsyncdPage() {
  const { t } = useTranslation()
  const [status, setStatus] = useState('')
  const [tasks, setTasks] = useState<AsyncdTask[] | null>(null)
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [schedules, setSchedules] = useState<AsyncdSchedule[]>([])

  const load = useCallback(async () => {
    try {
      const [taskPage, schedulePage] = await Promise.all([
        asyncdApi.tasks({ status, size: 50 }),
        asyncdApi.schedules(),
      ])
      setTasks(taskPage.items ?? [])
      setCounts(taskPage.counts ?? {})
      setSchedules(schedulePage.items ?? [])
    } catch (error) {
      toast.error(describeError(error, t))
      setTasks([])
    }
  }, [status, t])

  useEffect(() => {
    void load()
  }, [load])

  const retry = async (task: AsyncdTask) => {
    try {
      await asyncdApi.retry(task.id)
      toast.success(t('asyncd.retried'))
      await load()
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  const toggleSchedule = async (schedule: AsyncdSchedule, enabled: boolean) => {
    try {
      await asyncdApi.setScheduleEnabled(schedule.job_name, enabled)
      await load()
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  return (
    <PageShell titleKey="asyncd.title" subtitleKey="asyncd.subtitle">
      <div className="space-y-5">
        <SummaryStrip
          items={[
            { label: t('asyncd.statusPending'), value: counts.pending ?? 0 },
            { label: t('asyncd.statusActive'), value: counts.active ?? 0, tone: 'warning' },
            { label: t('asyncd.statusCompleted'), value: counts.completed ?? 0, tone: 'success' },
            {
              label: t('asyncd.statusArchived'),
              value: counts.archived ?? 0,
              tone: (counts.archived ?? 0) > 0 ? 'danger' : 'default',
            },
          ]}
        />

        <div className="flex flex-wrap items-center justify-between gap-3">
          <Tabs value={status || 'all'} onValueChange={(value) => setStatus(value === 'all' ? '' : value)}>
            <TabsList>
              <TabsTrigger value="all">{t('asyncd.filterAll')}</TabsTrigger>
              <TabsTrigger value="pending">{t('asyncd.statusPending')}</TabsTrigger>
              <TabsTrigger value="active">{t('asyncd.statusActive')}</TabsTrigger>
              <TabsTrigger value="archived">{t('asyncd.statusArchived')}</TabsTrigger>
            </TabsList>
          </Tabs>
          <Button variant="outline" size="sm" onClick={() => void load()}>
            <RefreshCw className="size-4" /> {t('common.refresh')}
          </Button>
        </div>

        {tasks === null ? (
          <Skeleton className="h-40 w-full" />
        ) : tasks.length === 0 ? (
          <div className="bg-card border-border/70 shadow-soft rounded-2xl border p-8 text-center">
            <p className="text-sm font-medium">{t('asyncd.empty')}</p>
            <p className="text-muted-foreground mt-1 text-xs">{t('asyncd.emptyHint')}</p>
          </div>
        ) : (
          <div className="divide-border/60 divide-y rounded-2xl border border-border/70 shadow-soft">
            {tasks.map((task) => (
              <div key={task.id} className="flex flex-wrap items-start gap-x-6 gap-y-2 p-4">
                <div className="min-w-56 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-[13px] font-medium">{task.type}</span>
                    <Badge
                      variant={
                        task.status === 'completed'
                          ? 'success'
                          : task.status === 'archived'
                            ? 'destructive'
                            : 'secondary'
                      }
                    >
                      {STATUS_KEYS[task.status] ? t(STATUS_KEYS[task.status]) : task.status}
                    </Badge>
                    <span className="text-muted-foreground text-[11px]">
                      {t('asyncd.attempts', { attempts: task.attempts, max: task.max_attempts })}
                    </span>
                    {task.unique_key ? (
                      <span className="text-muted-foreground font-mono text-[11px]">{task.unique_key}</span>
                    ) : null}
                  </div>
                  <p className="text-muted-foreground text-[11px]">
                    {t('asyncd.nextAttempt', { value: formatRelative(task.next_attempt_at) })}
                    {task.locked_by ? ` · ${task.locked_by}` : ''}
                    {task.institution_id ? ` · #${task.institution_id}` : ''}
                  </p>
                  {task.last_error ? (
                    <p className="text-destructive truncate text-[11px]">{task.last_error}</p>
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <span className="text-muted-foreground text-[11px]">{formatDateTime(task.created_at)}</span>
                  {task.status === 'archived' ? (
                    <Button size="sm" variant="outline" onClick={() => void retry(task)}>
                      <RotateCcw className="size-3.5" /> {t('asyncd.retry')}
                    </Button>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        )}

        <div className="space-y-3">
          <div>
            <p className="text-[13px] font-semibold">{t('asyncd.schedules')}</p>
            <p className="text-muted-foreground text-[11px]">{t('asyncd.schedulesHint')}</p>
          </div>
          <div className="divide-border/60 divide-y rounded-2xl border border-border/70 shadow-soft">
            {schedules.map((schedule) => (
              <div key={schedule.job_name} className="flex flex-wrap items-center gap-x-6 gap-y-2 p-4">
                <div className="min-w-56 flex-1 space-y-1">
                  <p className="font-mono text-[13px]">{schedule.job_name}</p>
                  <p className="text-muted-foreground text-[11px]">
                    {schedule.kind === 'daily'
                      ? t('asyncd.scheduleDaily', {
                          times: (schedule.daily_times ?? []).join(' / ') || '-',
                          window: schedule.window_minutes,
                        })
                      : t('asyncd.scheduleInterval', { seconds: schedule.interval_seconds })}
                    {` · ${t('asyncd.nextRun', { value: formatRelative(schedule.next_run_at) })}`}
                    {schedule.last_run_at ? ` · ${t('asyncd.lastRun', { value: formatRelative(schedule.last_run_at) })}` : ''}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  {schedule.last_status !== 'idle' ? (
                    <Badge variant={schedule.last_status === 'enqueued' ? 'success' : 'secondary'}>
                      {schedule.last_status}
                    </Badge>
                  ) : null}
                  <Switch
                    checked={schedule.enabled}
                    onCheckedChange={(checked) => void toggleSchedule(schedule, checked)}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </PageShell>
  )
}
