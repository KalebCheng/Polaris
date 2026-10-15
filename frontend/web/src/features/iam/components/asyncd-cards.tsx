import { useTranslation } from 'react-i18next'
import { CalendarClock, RotateCcw, Timer } from 'lucide-react'
import { Badge, Button, Switch } from '@/shared/ui'
import { formatDateTime, formatRelative } from '@/shared/lib/format'
import type { AsyncdSchedule, AsyncdTask } from '../api'
import { RESOURCE_CARD_CLASS } from './resource-cards'

/** 任务状态 → 文案 key（与任务中心的状态筛选共用同一套词汇） */
export const ASYNCD_STATUS_KEYS: Record<string, string> = {
  pending: 'asyncd.statusPending',
  active: 'asyncd.statusActive',
  completed: 'asyncd.statusCompleted',
  archived: 'asyncd.statusArchived',
}

/** 状态徽标语气：完成=成功、死信=危险、执行中/等待中=中性 */
function statusVariant(status: string): 'success' | 'destructive' | 'secondary' {
  if (status === 'completed') return 'success'
  if (status === 'archived') return 'destructive'
  return 'secondary'
}

/**
 * 异步任务卡片：一眼看清"这是什么任务、卡在哪一步、要不要人工介入"。
 *
 * 形态与资源卡片一致（`RESOURCE_CARD_CLASS`：等高等宽、页脚贴底），
 * 因为任务列表与组织/应用列表是同一类心智："逐个识别一个对象并处置它"。
 */
export function AsyncdTaskCard({ task, onRetry }: { task: AsyncdTask; onRetry: () => void }) {
  const { t } = useTranslation()
  const statusKey = ASYNCD_STATUS_KEYS[task.status]

  return (
    <article className={RESOURCE_CARD_CLASS}>
      <header className="flex items-start gap-3">
        <span className="bg-primary/10 text-primary grid size-10 shrink-0 place-items-center rounded-[10px]">
          <Timer className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate font-mono text-sm font-semibold">{task.type}</p>
          <div className="mt-1.5 flex flex-wrap items-center gap-1">
            <Badge variant={statusVariant(task.status)}>{statusKey ? t(statusKey) : task.status}</Badge>
            <span className="text-muted-foreground text-[11px]">
              {t('asyncd.attempts', { attempts: task.attempts, max: task.max_attempts })}
            </span>
          </div>
        </div>
      </header>

      <dl className="space-y-1 text-[11px]">
        {task.unique_key ? (
          <div className="flex items-center gap-2">
            <dt className="text-muted-foreground shrink-0">{t('asyncd.uniqueKey')}</dt>
            <dd className="truncate font-mono">{task.unique_key}</dd>
          </div>
        ) : null}
        <div className="flex items-center gap-2">
          <dt className="text-muted-foreground shrink-0">{t('asyncd.nextAttemptLabel')}</dt>
          <dd className="truncate">{formatRelative(task.next_attempt_at)}</dd>
        </div>
        {task.institution_id ? (
          <div className="flex items-center gap-2">
            <dt className="text-muted-foreground shrink-0">{t('common.institution')}</dt>
            <dd className="truncate">#{task.institution_id}</dd>
          </div>
        ) : null}
        {task.locked_by ? (
          <div className="flex items-center gap-2">
            <dt className="text-muted-foreground shrink-0">{t('asyncd.lockedBy')}</dt>
            <dd className="truncate font-mono">{task.locked_by}</dd>
          </div>
        ) : null}
      </dl>

      {task.last_error ? (
        <p className="bg-destructive/10 text-destructive line-clamp-3 rounded-lg px-2.5 py-1.5 text-[11px] break-all">
          {task.last_error}
        </p>
      ) : null}

      <footer className="border-border/60 mt-auto flex items-center justify-between gap-2 border-t pt-3">
        <span className="text-muted-foreground truncate text-[11px]">
          {t('common.createdAt')} · {formatDateTime(task.created_at)}
        </span>
        {/* 只有死信才有"重试"这个动作：其他状态的介入手段是等待或看日志 */}
        {task.status === 'archived' ? (
          <Button size="sm" variant="outline" onClick={onRetry}>
            <RotateCcw className="size-3.5" /> {t('asyncd.retry')}
          </Button>
        ) : null}
      </footer>
    </article>
  )
}

/** 定时调度卡片：名称 + 频率 + 下次执行 + 启停开关 */
export function AsyncdScheduleCard({
  schedule,
  onToggle,
}: {
  schedule: AsyncdSchedule
  onToggle: (enabled: boolean) => void
}) {
  const { t } = useTranslation()
  const frequency =
    schedule.kind === 'daily'
      ? t('asyncd.scheduleDaily', {
          times: (schedule.daily_times ?? []).join(' / ') || '-',
          window: schedule.window_minutes,
        })
      : t('asyncd.scheduleInterval', { seconds: schedule.interval_seconds })

  return (
    <article className={RESOURCE_CARD_CLASS}>
      <header className="flex items-start gap-3">
        <span className="bg-primary/10 text-primary grid size-10 shrink-0 place-items-center rounded-[10px]">
          <CalendarClock className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate font-mono text-sm font-semibold">{schedule.job_name}</p>
          <p className="text-muted-foreground mt-1.5 text-[11px]">{frequency}</p>
        </div>
        <Switch checked={schedule.enabled} onCheckedChange={(checked) => onToggle(checked)} />
      </header>

      <dl className="space-y-1 text-[11px]">
        <div className="flex items-center gap-2">
          <dt className="text-muted-foreground shrink-0">{t('asyncd.nextRunLabel')}</dt>
          <dd className="truncate">{formatRelative(schedule.next_run_at)}</dd>
        </div>
      </dl>

      <footer className="border-border/60 mt-auto flex items-center justify-between gap-2 border-t pt-3">
        <span className="text-muted-foreground truncate text-[11px]">
          {t('asyncd.lastRunLabel')} · {schedule.last_run_at ? formatRelative(schedule.last_run_at) : '—'}
        </span>
        {schedule.last_status !== 'idle' ? (
          <Badge variant={schedule.last_status === 'enqueued' ? 'success' : 'secondary'}>
            {schedule.last_status}
          </Badge>
        ) : null}
      </footer>
    </article>
  )
}
