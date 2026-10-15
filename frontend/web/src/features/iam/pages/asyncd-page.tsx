import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { RefreshCw } from 'lucide-react'
import { Button, Tabs, TabsList, TabsTrigger } from '@/shared/ui'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { describeError } from '@/shared/lib/errors'
import { asyncdApi, type AsyncdJob, type AsyncdWorker } from '../api'
import { AsyncdJobCard } from '../components/asyncd-job-card'
import { AsyncdRunsDialog } from '../components/asyncd-runs-dialog'
import { AsyncdWorkers } from '../components/asyncd-workers'
import { ResourceCards } from '../components/resource-cards'

/** 任务中心的两层：任务维度（异步 / 定时）与执行器维度 */
type Dimension = 'async' | 'scheduled' | 'workers'

/**
 * asyncd 任务中心（平台级）：四个维度都能看见。
 *
 *   任务定义 → 异步任务 / 定时任务（任务维度，含从未执行过的任务）
 *   执行实例 → 点进任务的「执行记录」列表（一次执行一行）
 *   执行记录 → 某个执行实例的尝试明细（第几次尝试、耗时、错误）
 *   执行器   → 有几个服务/槽位在跑、谁失联了、各自在跑几个任务
 *
 * 为什么必须分两层：`asyncd_job_runs` 直接铺成列表看到的是"同一任务的第 N 次执行"，
 * 看不出任务层面的堆积与失败比例；而只看任务卡片又答不了"这次到底为什么失败"。
 */
export default function AsyncdPage() {
  const { t } = useTranslation()
  const [dimension, setDimension] = useState<Dimension>('async')
  const [jobs, setJobs] = useState<AsyncdJob[] | null>(null)
  const [workers, setWorkers] = useState<AsyncdWorker[]>([])
  const [selected, setSelected] = useState<AsyncdJob | null>(null)

  const load = useCallback(async () => {
    try {
      const [jobList, workerList] = await Promise.all([asyncdApi.jobs(), asyncdApi.workers()])
      setJobs(jobList.items ?? [])
      setWorkers(workerList.items ?? [])
    } catch (error) {
      toast.error(describeError(error, t))
      setJobs([])
    }
  }, [t])

  useEffect(() => {
    void load()
  }, [load])

  const toggleJob = async (job: AsyncdJob, enabled: boolean) => {
    try {
      await asyncdApi.setJobEnabled(job.id, enabled)
      await load()
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  /**
   * 队列健康度直接从任务统计求和：每个执行实例都有 job_id（NOT NULL），
   * 而目录列出的是**全部**任务定义，因此这个求和是全量的，无需再查一次列表接口。
   */
  const queue = (jobs ?? []).reduce<Record<string, number>>((acc, job) => {
    for (const [status, count] of Object.entries(job.stats.by_status ?? {})) {
      acc[status] = (acc[status] ?? 0) + count
    }
    return acc
  }, {})

  const visible = (jobs ?? []).filter((job) =>
    dimension === 'scheduled' ? job.kind === 'scheduled' : job.kind === 'async',
  )
  const emptyKey = dimension === 'scheduled' ? 'asyncd.scheduledEmpty' : 'asyncd.asyncEmpty'
  const emptyHintKey = dimension === 'scheduled' ? 'asyncd.scheduledEmptyHint' : 'asyncd.asyncEmptyHint'
  const hintKey =
    dimension === 'workers'
      ? 'asyncd.workersHint'
      : dimension === 'scheduled'
        ? 'asyncd.scheduledHint'
        : 'asyncd.asyncHint'

  return (
    <div className="space-y-5">
      <SummaryStrip
        items={[
          { label: t('asyncd.statusQueued'), value: queue.queued ?? 0 },
          { label: t('asyncd.statusRunning'), value: queue.running ?? 0, tone: 'warning' },
          { label: t('asyncd.statusSucceeded'), value: queue.succeeded ?? 0, tone: 'success' },
          {
            label: t('asyncd.statusDead'),
            value: queue.dead ?? 0,
            tone: (queue.dead ?? 0) > 0 ? 'danger' : 'default',
          },
        ]}
      />

      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs value={dimension} onValueChange={(value) => setDimension(value as Dimension)}>
          <TabsList>
            <TabsTrigger value="async">{t('asyncd.tabAsync')}</TabsTrigger>
            <TabsTrigger value="scheduled">{t('asyncd.tabScheduled')}</TabsTrigger>
            <TabsTrigger value="workers">{t('asyncd.tabWorkers')}</TabsTrigger>
          </TabsList>
        </Tabs>
        <Button variant="outline" size="sm" onClick={() => void load()}>
          <RefreshCw className="size-4" /> {t('common.refresh')}
        </Button>
      </div>

      <p className="text-muted-foreground text-[11px]">{t(hintKey)}</p>

      {dimension === 'workers' ? (
        <AsyncdWorkers workers={workers} loading={jobs === null} />
      ) : (
        // 任务是**代码里声明的封闭集合**（定义表），不会无限增长，故不分页
        <ResourceCards
          items={visible}
          cardKey={(job) => job.id}
          loading={jobs === null}
          page={1}
          size={Math.max(visible.length, 1)}
          total={visible.length}
          onPageChange={() => undefined}
          emptyTitle={t(emptyKey)}
          emptyDescription={t(emptyHintKey)}
          unit={t('asyncd.jobsUnit')}
          renderCard={(job) => (
            <AsyncdJobCard
              job={job}
              onOpenRuns={() => setSelected(job)}
              onToggle={(enabled) => void toggleJob(job, enabled)}
            />
          )}
        />
      )}

      {/* key 绑定任务主键：换任务时弹窗内的筛选/分页/选中行一律回到初始状态 */}
      <AsyncdRunsDialog
        key={selected?.id ?? 'none'}
        job={selected}
        onClose={() => setSelected(null)}
        onChanged={() => void load()}
      />
    </div>
  )
}
