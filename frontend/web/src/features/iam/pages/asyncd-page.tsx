import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { RefreshCw } from 'lucide-react'
import { Button, Tabs, TabsList, TabsTrigger } from '@/shared/ui'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { describeError } from '@/shared/lib/errors'
import { asyncdApi, type AsyncdSchedule, type AsyncdTask } from '../api'
import { AsyncdScheduleCard, AsyncdTaskCard } from '../components/asyncd-cards'
import { ResourceCards } from '../components/resource-cards'

/** 卡片网格每页条数（3–4 列时刚好铺满整数行） */
const PAGE_SIZE = 12

/**
 * asyncd 任务中心（平台级）：异步任务与定时调度的可见性与人工干预。
 *
 * 为什么需要这个页面：队列类问题的通用解法是**可观测 + 可干预**——
 * 只看日志无法回答"有没有堆积、为什么失败、要不要重试"，而"慢操作"恰恰最需要这些。
 *
 * 列表形态与组织管理下的资源页一致（筛选行 + **卡片网格** + 分页）：任务与组织/应用同属
 * "逐个识别一个对象再处置它"的心智，摊成一行行反而看不出"这是个什么任务、卡在哪一步"。
 */
export default function AsyncdPage() {
  const { t } = useTranslation()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [tasks, setTasks] = useState<AsyncdTask[] | null>(null)
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [schedules, setSchedules] = useState<AsyncdSchedule[]>([])

  const load = useCallback(async () => {
    try {
      const [taskPage, schedulePage] = await Promise.all([
        asyncdApi.tasks({ status, page, size: PAGE_SIZE }),
        asyncdApi.schedules(),
      ])
      setTasks(taskPage.items ?? [])
      setCounts(taskPage.counts ?? {})
      setSchedules(schedulePage.items ?? [])
    } catch (error) {
      toast.error(describeError(error, t))
      setTasks([])
    }
  }, [status, page, t])

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

  /**
   * 分页总数由后端下发的**状态直方图**派生：`counts` 与列表同一作用域（同一 type 过滤），
   * 所以"全部"＝各状态求和、"某状态"＝该状态的计数——分页器不必再多一个 total 字段。
   */
  const total = status
    ? (counts[status] ?? 0)
    : Object.values(counts).reduce((sum, value) => sum + value, 0)

  return (
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

      <ResourceCards
        items={tasks ?? []}
        cardKey={(task) => task.id}
        loading={tasks === null}
        page={page}
        size={PAGE_SIZE}
        total={total}
        onPageChange={setPage}
        emptyTitle={t('asyncd.empty')}
        emptyDescription={t('asyncd.emptyHint')}
        renderCard={(task) => <AsyncdTaskCard task={task} onRetry={() => void retry(task)} />}
        toolbar={
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Tabs
              value={status || 'all'}
              onValueChange={(value) => {
                setStatus(value === 'all' ? '' : value)
                setPage(1) // 换筛选条件必须回到第 1 页，否则会停在越界页码上显示空列表
              }}
            >
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
        }
      />

      <div className="space-y-3">
        <div>
          <p className="text-[13px] font-semibold">{t('asyncd.schedules')}</p>
          <p className="text-muted-foreground text-[11px]">{t('asyncd.schedulesHint')}</p>
        </div>
        {schedules.length === 0 ? (
          <p className="text-muted-foreground text-xs">{t('asyncd.schedulesEmpty')}</p>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            {schedules.map((schedule) => (
              <AsyncdScheduleCard
                key={schedule.job_name}
                schedule={schedule}
                onToggle={(enabled) => void toggleSchedule(schedule, enabled)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
