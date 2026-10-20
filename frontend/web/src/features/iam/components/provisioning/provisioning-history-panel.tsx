import { useTranslation } from 'react-i18next'
import { Badge } from '@/shared/ui'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { formatDateTime } from '@/shared/lib/format'
import type { ProvisionHistory } from '@/shared/api/types'
import { provisionHistoryApi } from '../../api'
import { useListQuery } from '../../hooks/use-list-query'
import { ResourceTable, type Column } from '../resource-table'

const actionLabelKeys: Record<string, string> = {
  create: 'provisioning.actionCreate',
  update: 'provisioning.actionUpdate',
  disable: 'provisioning.actionDisable',
  delete: 'provisioning.actionDelete',
  retry: 'provisioning.actionRetry',
}

interface ProvisioningHistoryPanelProps {
  institution: string
  applicationId: number
  applicationCode: string
}

/** 应用详情内的账号同步记录（只读）：建号 / 更新 / 禁用 / 删除 / 重试的流水。 */
export function ProvisioningHistoryPanel({
  institution,
  applicationId,
  applicationCode,
}: ProvisioningHistoryPanelProps) {
  const { t } = useTranslation()
  const list = useListQuery<ProvisionHistory>((params) => provisionHistoryApi.list(params), {
    institution: institution || undefined,
    filters: { application_id: applicationId },
  })

  const columns: Column<ProvisionHistory>[] = [
    {
      key: 'action',
      headerKey: 'provisioning.action',
      render: (record) => (
        <Badge variant="secondary">{t(actionLabelKeys[record.action] ?? 'provisioning.action')}</Badge>
      ),
    },
    {
      key: 'status',
      headerKey: 'provisioning.status',
      render: (record) => (
        <Badge
          variant={
            record.status === 'success' ? 'success' : record.status === 'failed' ? 'destructive' : 'outline'
          }
        >
          {record.status}
        </Badge>
      ),
    },
    {
      key: 'user',
      headerKey: 'provisioning.user',
      render: (record) => <span className="text-sm">{record.user || '-'}</span>,
    },
    {
      key: 'connector',
      headerKey: 'provisioning.connector',
      render: (record) => <span className="text-sm">{record.connector || '-'}</span>,
    },
    {
      key: 'message',
      headerKey: 'provisioning.message',
      render: (record) => (
        <span className="text-muted-foreground line-clamp-1 max-w-[20rem] text-xs">
          {record.message || '-'}
        </span>
      ),
    },
    {
      key: 'createdAt',
      headerKey: 'common.createdAt',
      render: (record) => <span className="text-xs">{formatDateTime(record.created_at)}</span>,
    },
  ]

  return (
    <div className="space-y-4">
      <div className="border-border/60 bg-muted/30 rounded-xl border px-3.5 py-3">
        <p className="text-sm font-medium">{t('provisioning.applicationScopedTitle')}</p>
        <p className="text-muted-foreground mt-1 text-xs leading-relaxed">
          {t('provisioning.applicationScopedHint', { application: applicationCode })}
        </p>
      </div>

      <ListToolbar keyword={list.keyword} onKeywordChange={list.setKeyword} onSubmit={list.search} />

      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(record) => record.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
      />
    </div>
  )
}
