import { useTranslation } from 'react-i18next'
import { Badge } from '@/shared/ui'
import { formatRelative } from '@/shared/lib/format'
import type { AsyncdWorker } from '../api'
import { ASYNCD_WORKER_STATE_KEYS, asyncdWorkerStateVariant } from '../lib/asyncd-status'
import { ResourceTable, type Column } from './resource-table'

/**
 * 执行器实例表格：有几个服务在跑、各自几个槽位、谁已失联、各自在跑几个任务。
 *
 * 在线状态由后端按**心跳新鲜度**算出（online / stale / stopped）——进程被 kill 后
 * 约 30s 内会从 online 变 stale，因此这里能如实反映"谁掉线了"。
 */
export function AsyncdWorkers({ workers, loading }: { workers: AsyncdWorker[]; loading: boolean }) {
  const { t } = useTranslation()

  const columns: Column<AsyncdWorker>[] = [
    {
      key: 'instance',
      headerKey: 'asyncd.colInstance',
      render: (row) => <span className="font-mono text-[12px]">{row.instance_id}</span>,
    },
    {
      key: 'slot',
      headerKey: 'asyncd.colSlot',
      render: (row) => <span className="font-mono text-[12px]">#{row.slot_index}</span>,
    },
    {
      key: 'state',
      headerKey: 'asyncd.colState',
      render: (row) => (
        <Badge variant={asyncdWorkerStateVariant(row.state)}>
          {ASYNCD_WORKER_STATE_KEYS[row.state] ? t(ASYNCD_WORKER_STATE_KEYS[row.state]) : row.state}
        </Badge>
      ),
    },
    {
      key: 'running',
      headerKey: 'asyncd.colRunning',
      render: (row) => <span className="tabular-nums">{row.running}</span>,
    },
    {
      key: 'started',
      headerKey: 'asyncd.colStartedAt',
      render: (row) => <span className="whitespace-nowrap text-[12px]">{formatRelative(row.started_at)}</span>,
    },
    {
      key: 'heartbeat',
      headerKey: 'asyncd.colHeartbeat',
      render: (row) => (
        <span className="whitespace-nowrap text-[12px]">{formatRelative(row.last_heartbeat_at)}</span>
      ),
    },
  ]

  return (
    <ResourceTable
      columns={columns}
      rows={workers}
      rowKey={(row) => row.id}
      loading={loading}
      page={1}
      size={Math.max(workers.length, 1)}
      total={workers.length}
      onPageChange={() => undefined}
      emptyTitle={t('asyncd.workersEmpty')}
      emptyDescription={t('asyncd.workersEmptyHint')}
      unit={t('asyncd.workersUnit')}
    />
  )
}
