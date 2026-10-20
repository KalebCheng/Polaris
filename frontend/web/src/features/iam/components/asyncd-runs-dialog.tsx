import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/shared/ui/toast'
import { RotateCcw, Timer } from 'lucide-react'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Tabs,
  TabsList,
  TabsTrigger,
} from '@/shared/ui'
import { formatDateTime } from '@/shared/lib/format'
import { describeError } from '@/shared/lib/errors'
import { asyncdApi, type AsyncdJob, type AsyncdJobRun, type AsyncdJobRunAttempt } from '../api'
import {
  ASYNCD_ATTEMPT_STATUS_KEYS,
  ASYNCD_RUN_STATUS_KEYS,
  asyncdAttemptStatusVariant,
  asyncdRunStatusVariant,
  formatDuration,
} from '../lib/asyncd-status'
import { ResourceTable, type Column } from './resource-table'

/** 执行记录每页条数 */
const RUNS_PAGE_SIZE = 10

interface AsyncdRunsDialogProps {
  /** 当前查看记录的任务；null 表示关闭 */
  job: AsyncdJob | null
  onClose: () => void
  /** 人工重试死信后回调（任务统计随之变化，列表需要刷新） */
  onChanged: () => void
}

/**
 * 执行记录：某个任务的历史执行（执行实例）列表 + 某次执行的**尝试明细**。
 *
 * 两层信息各答一个问题：
 *   - 执行实例（一行 = 一次执行）：这次执行最终成功了吗、现在还在等吗、重试到第几次了；
 *   - 尝试明细（一行 = 一次尝试）：第几次尝试失败、耗时多久、报的什么错——
 *     重试会覆盖执行实例上的 last_error，所以这份明细只能单独记录。
 */
export function AsyncdRunsDialog({ job, onClose, onChanged }: AsyncdRunsDialogProps) {
  const { t } = useTranslation()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [runs, setRuns] = useState<AsyncdJobRun[] | null>(null)
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [detail, setDetail] = useState<AsyncdJobRun | null>(null)
  const [attempts, setAttempts] = useState<AsyncdJobRunAttempt[] | null>(null)

  const load = useCallback(async () => {
    if (!job) return
    try {
      const result = await asyncdApi.jobRuns(job.id, { status, page, size: RUNS_PAGE_SIZE })
      setRuns(result.items ?? [])
      setCounts(result.counts ?? {})
    } catch (error) {
      toast.error(describeError(error, t))
      setRuns([])
    }
  }, [job, status, page, t])

  useEffect(() => {
    void load()
  }, [load])

  // 尝试明细按需加载：列表本身不查明细，避免 N+1
  useEffect(() => {
    if (!detail) {
      setAttempts(null)
      return
    }
    let cancelled = false
    void (async () => {
      try {
        const result = await asyncdApi.jobRunAttempts(job ? job.id : 0, detail.id)
        if (!cancelled) setAttempts(result.items ?? [])
      } catch (error) {
        if (!cancelled) {
          toast.error(describeError(error, t))
          setAttempts([])
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [detail, job, t])

  const retry = async (run: AsyncdJobRun) => {
    if (!job) return
    try {
      await asyncdApi.retryJobRun(job.id, run.id)
      toast.success(t('asyncd.retried'))
      await load()
      onChanged()
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  /** 分页总数由状态直方图派生（`counts` 与列表同一作用域） */
  const total = status ? (counts[status] ?? 0) : Object.values(counts).reduce((sum, value) => sum + value, 0)

  const columns: Column<AsyncdJobRun>[] = [
    {
      key: 'status',
      headerKey: 'asyncd.colStatus',
      render: (row) => (
        <Badge variant={asyncdRunStatusVariant(row.status)}>
          {ASYNCD_RUN_STATUS_KEYS[row.status] ? t(ASYNCD_RUN_STATUS_KEYS[row.status]) : row.status}
        </Badge>
      ),
    },
    {
      key: 'attempts',
      headerKey: 'asyncd.colAttempts',
      render: (row) => (
        <span className="tabular-nums">
          {t('asyncd.attempts', { attempts: row.attempts, max: row.max_attempts })}
        </span>
      ),
    },
    {
      key: 'queued',
      headerKey: 'asyncd.colQueued',
      render: (row) => (
        <span className="whitespace-nowrap text-[12px]">{formatDateTime(row.created_at)}</span>
      ),
    },
    {
      key: 'finished',
      headerKey: 'asyncd.colFinished',
      render: (row) => (
        <span className="whitespace-nowrap text-[12px]">
          {row.finished_at ? formatDateTime(row.finished_at) : '—'}
        </span>
      ),
    },
    {
      key: 'worker',
      headerKey: 'asyncd.colWorker',
      render: (row) => <span className="font-mono text-[11px]">{row.worker_id || '—'}</span>,
    },
    {
      key: 'error',
      headerKey: 'asyncd.colLastError',
      className: 'max-w-[18rem]',
      render: (row) =>
        row.last_error ? (
          <span className="text-destructive block truncate text-[11px]" title={row.last_error}>
            {row.last_error}
          </span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      key: 'actions',
      headerKey: 'asyncd.colActions',
      render: (row) => (
        <div className="flex items-center gap-1.5">
          <Button size="sm" variant="ghost" onClick={() => setDetail(row)}>
            <Timer className="size-3.5" /> {t('asyncd.attemptDetail')}
          </Button>
          {/* 只有死信才有"重试"这个动作：其他状态的介入手段是等待或看日志 */}
          {row.status === 'dead' ? (
            <Button size="sm" variant="outline" onClick={() => void retry(row)}>
              <RotateCcw className="size-3.5" /> {t('asyncd.retry')}
            </Button>
          ) : null}
        </div>
      ),
    },
  ]

  return (
    <Dialog
      open={job !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle className="font-mono">{job?.name}</DialogTitle>
          <DialogDescription>
            {job?.kind === 'scheduled'
              ? t('asyncd.runsScheduledHint', { name: job?.task_type ?? '' })
              : t('asyncd.runsAsyncHint', { name: job?.task_type ?? '' })}
          </DialogDescription>
        </DialogHeader>

        <Tabs
          value={status || 'all'}
          onValueChange={(value) => {
            setStatus(value === 'all' ? '' : value)
            setPage(1) // 换筛选条件必须回到第 1 页，否则会停在越界页码上显示空列表
            setDetail(null)
          }}
        >
          <TabsList>
            <TabsTrigger value="all">{t('asyncd.filterAll')}</TabsTrigger>
            {Object.entries(ASYNCD_RUN_STATUS_KEYS).map(([value, key]) => (
              <TabsTrigger key={value} value={value}>
                {t(key)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <ResourceTable
          columns={columns}
          rows={runs ?? []}
          rowKey={(row) => row.id}
          loading={runs === null}
          page={page}
          size={RUNS_PAGE_SIZE}
          total={total}
          onPageChange={setPage}
          emptyTitle={t('asyncd.runsEmpty')}
          emptyDescription={t('asyncd.runsEmptyHint')}
          unit={t('asyncd.runsUnit')}
        />

        {/* 尝试明细：某次执行里第 1/2/3 次尝试各自的结果与耗时 */}
        {detail ? (
          <section className="space-y-2">
            <p className="text-[13px] font-semibold">{t('asyncd.attemptDetailTitle', { id: detail.id })}</p>
            {attempts === null ? (
              <p className="text-muted-foreground text-xs">{t('common.loading')}</p>
            ) : attempts.length === 0 ? (
              <p className="text-muted-foreground text-xs">{t('asyncd.attemptsEmpty')}</p>
            ) : (
              <ul className="divide-border/60 divide-y rounded-xl border text-[12px]">
                {attempts.map((attempt) => (
                  <li key={attempt.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-2">
                    <span className="text-muted-foreground w-16 shrink-0">
                      {t('asyncd.attemptNo', { no: attempt.attempt_no })}
                    </span>
                    <Badge variant={asyncdAttemptStatusVariant(attempt.status)}>
                      {ASYNCD_ATTEMPT_STATUS_KEYS[attempt.status]
                        ? t(ASYNCD_ATTEMPT_STATUS_KEYS[attempt.status])
                        : attempt.status}
                    </Badge>
                    <span className="text-muted-foreground">{formatDateTime(attempt.started_at)}</span>
                    <span className="font-mono tabular-nums">{formatDuration(attempt.duration_ms)}</span>
                    <span className="text-muted-foreground font-mono">{attempt.worker_id}</span>
                    {attempt.error ? (
                      <span className="text-destructive w-full break-all" title={attempt.error}>
                        {attempt.error}
                      </span>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
          </section>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
