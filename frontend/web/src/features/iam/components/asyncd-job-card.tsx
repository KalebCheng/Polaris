import { useTranslation } from 'react-i18next'
import { CalendarClock, ListTree, Timer } from 'lucide-react'
import { Badge, Button, Switch } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'
import { formatRelative } from '@/shared/lib/format'
import type { AsyncdJob } from '../api'
import { ASYNCD_RUN_STATUS_KEYS, ASYNCD_RUN_STATUS_ORDER } from '../lib/asyncd-status'
import type { TFunction } from 'i18next'
import { RESOURCE_CARD_CLASS } from './resource-cards'

/** 调度规则的展示口径（interval / daily 窗口 / cron 表达式），任务卡与测试共用一处 */
export function scheduleFrequency(schedule: NonNullable<AsyncdJob['schedule']>, t: TFunction): string {
  if (schedule.kind === 'cron') {
    return t('asyncd.scheduleCron', { expr: schedule.cron_expr || '-' })
  }
  if (schedule.kind === 'daily') {
    return t('asyncd.scheduleDaily', {
      times: (schedule.daily_times ?? []).join(' / ') || '-',
      window: schedule.window_minutes,
    })
  }
  return t('asyncd.scheduleInterval', { seconds: schedule.interval_seconds })
}

interface AsyncdJobCardProps {
  job: AsyncdJob
  /** 打开该任务的执行记录 */
  onOpenRuns: () => void
  /** 定时任务专有：启停调度（异步任务不渲染开关） */
  onToggle: (enabled: boolean) => void
}

/**
 * 任务卡片：一个「任务定义」（异步任务 = 按类型注册；定时任务 = 调度声明）。
 *
 * 卡片回答"任务是谁、总共跑了多少、最近跑得怎么样"；单次执行的明细
 * （重试了几次、每次耗时与错误、谁跑的）在「执行记录」里看。
 */
export function AsyncdJobCard({ job, onOpenRuns, onToggle }: AsyncdJobCardProps) {
  const { t } = useTranslation()
  const schedule = job.schedule
  const frequency = schedule ? scheduleFrequency(schedule, t) : ''
  // 归属身份连接的任务：启停跟随连接的计划（这里不提供开关，避免两处真相）
  const ownedByConnection = job.owner_syncer_id != null

  return (
    <article className={RESOURCE_CARD_CLASS}>
      <header className="flex items-start gap-3">
        <span className="bg-primary/10 text-primary grid size-10 shrink-0 place-items-center rounded-[10px]">
          {job.kind === 'scheduled' ? <CalendarClock className="size-5" /> : <Timer className="size-5" />}
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate font-mono text-sm font-semibold">{job.name}</p>
          <div className="mt-1.5 flex flex-wrap items-center gap-1">
            <Badge variant="secondary">
              {job.kind === 'scheduled' ? t('asyncd.kindScheduled') : t('asyncd.kindAsync')}
            </Badge>
            {ownedByConnection ? (
              <Badge variant="outline">
                {t('asyncd.sourceSyncer')}
                {job.owner_name ? ` · ${job.owner_name}` : ''}
              </Badge>
            ) : null}
            {frequency ? <span className="text-muted-foreground text-[11px]">{frequency}</span> : null}
          </div>
        </div>
        {schedule && !ownedByConnection ? (
          <Switch
            checked={schedule.enabled}
            onCheckedChange={onToggle}
            aria-label={t('asyncd.toggleJob', { name: job.name })}
          />
        ) : null}
      </header>

      {/* 执行统计：四个状态一行两列，一眼看出"堆积在哪一步" */}
      <dl className="grid grid-cols-2 gap-2 text-[11px]">
        {ASYNCD_RUN_STATUS_ORDER.map((status) => {
          const count = job.stats.by_status?.[status] ?? 0
          return (
            <div key={status} className="bg-muted/40 flex items-center justify-between gap-2 rounded-lg px-2.5 py-1.5">
              <dt className="text-muted-foreground truncate">{t(ASYNCD_RUN_STATUS_KEYS[status])}</dt>
              <dd
                className={cn(
                  'font-mono tabular-nums',
                  status === 'dead' && count > 0 && 'text-destructive font-semibold',
                )}
              >
                {count}
              </dd>
            </div>
          )
        })}
      </dl>

      <dl className="space-y-1 text-[11px]">
        {schedule && ownedByConnection ? (
          <div className="flex items-center gap-2">
            <dt className="text-muted-foreground shrink-0">{t('asyncd.sourceLabel')}</dt>
            <dd className="truncate">
              {t('asyncd.ownedByConnection', { name: job.owner_name || `#${job.owner_syncer_id}` })}
            </dd>
          </div>
        ) : null}
        {schedule ? (
          <div className="flex items-center gap-2">
            <dt className="text-muted-foreground shrink-0">{t('asyncd.nextRunLabel')}</dt>
            <dd className="truncate">
              {schedule.enabled ? formatRelative(schedule.next_run_at) : t('asyncd.schedulePaused')}
            </dd>
          </div>
        ) : null}
        <div className="flex items-center gap-2">
          <dt className="text-muted-foreground shrink-0">{t('asyncd.lastQueuedLabel')}</dt>
          <dd className="truncate">
            {job.stats.last_queued_at ? formatRelative(job.stats.last_queued_at) : t('asyncd.neverRun')}
          </dd>
        </div>
      </dl>

      <footer className="border-border/60 mt-auto flex items-center justify-between gap-2 border-t pt-3">
        <span className="text-muted-foreground truncate text-[11px]">
          {t('asyncd.totalRuns', { total: job.stats.total })}
        </span>
        <Button size="sm" variant="outline" onClick={onOpenRuns}>
          <ListTree className="size-3.5" /> {t('asyncd.runs')}
        </Button>
      </footer>
    </article>
  )
}
