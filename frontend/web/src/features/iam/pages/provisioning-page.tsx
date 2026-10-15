import { useTranslation } from 'react-i18next'
import { Badge } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { formatDateTime } from '@/shared/lib/format'
import type { ProvisionHistory } from '@/shared/api/types'
import { provisionHistoryApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { ResourceTable, type Column } from '../components/resource-table'

const actionLabelKeys: Record<string, string> = {
  create: 'provisioning.actionCreate',
  update: 'provisioning.actionUpdate',
  disable: 'provisioning.actionDisable',
  delete: 'provisioning.actionDelete',
  retry: 'provisioning.actionRetry',
}

/** 供给历史：建号 / 更新 / 禁用 / 删除 / 重试的可追溯记录（只读） */
export default function ProvisioningPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()

  const list = useListQuery<ProvisionHistory>((params) => provisionHistoryApi.list(params), {
    institution: institution || undefined,
  })

  const columns: Column<ProvisionHistory>[] = [
    {
      key: 'action',
      headerKey: 'provisioning.action',
      render: (record) => <Badge variant="secondary">{t(actionLabelKeys[record.action] ?? 'provisioning.action')}</Badge>,
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
      key: 'application',
      headerKey: 'provisioning.application',
      render: (record) => <span className="text-sm">{record.application || '-'}</span>,
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
        <span className="text-muted-foreground line-clamp-1 max-w-[20rem] text-xs">{record.message || '-'}</span>
      ),
    },
    {
      key: 'createdAt',
      headerKey: 'common.createdAt',
      render: (record) => <span className="text-xs">{formatDateTime(record.created_at)}</span>,
    },
  ]

  return (
    <PageShell titleKey="provisioning.title" subtitleKey="provisioning.subtitle">
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(record) => record.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        toolbar={
          <ListToolbar keyword={list.keyword} onKeywordChange={list.setKeyword} onSubmit={list.search} />
        }
      />
    </PageShell>
  )
}
