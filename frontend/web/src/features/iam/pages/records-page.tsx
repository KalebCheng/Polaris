import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/shared/ui'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { StatusPill, type StatusTone } from '@/shared/components/status-pill'
import { formatDateTime } from '@/shared/lib/format'
import type { AuditRecord } from '@/shared/api/types'
import { recordApi } from '../api'
import { useListQuery } from '../hooks/use-list-query'
import { ResourceTable, type Column } from '../components/resource-table'

function resultTone(statusCode: number): StatusTone {
  if (statusCode >= 500) return 'danger'
  if (statusCode >= 400) return 'warning'
  return 'success'
}

/** 审计日志（平台级，跨组织）：检索认证、授权与管理操作记录 */
export default function RecordsPage() {
  const { t } = useTranslation()
  const list = useListQuery<AuditRecord>((params) => recordApi.list(params), { scope: 'platform' })
  const [detail, setDetail] = useState<AuditRecord | null>(null)

  const columns: Column<AuditRecord>[] = [
    {
      key: 'createdAt',
      headerKey: 'common.createdAt',
      className: 'w-44 text-xs',
      render: (record) => formatDateTime(record.created_at),
    },
    {
      key: 'institution',
      headerKey: 'common.institution',
      render: (record) => (record.institution_id ? <Badge variant="secondary">{String(record.institution_id)}</Badge> : '-'),
    },
    { key: 'user', headerKey: 'records.user', render: (record) => <span className="text-foreground">{record.user || '-'}</span> },
    {
      key: 'action',
      headerKey: 'records.action',
      render: (record) => <Badge variant="secondary">{record.action || '-'}</Badge>,
    },
    {
      key: 'object',
      headerKey: 'records.object',
      className: 'max-w-xs truncate text-xs',
      render: (record) => record.object || '-',
    },
    {
      key: 'result',
      headerKey: 'records.statusCode',
      render: (record) => (
        <StatusPill tone={resultTone(record.status_code)}>
          {record.status_code >= 400 ? t('records.resultFailed') : t('records.resultSuccess')}
        </StatusPill>
      ),
    },
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (record) => (
        <div className="flex justify-end">
          <Button variant="ghost" size="sm" onClick={() => setDetail(record)}>
            {t('records.detail')}
          </Button>
        </div>
      ),
    },
  ]

  return (
    <>
      <SummaryStrip
        items={[
          { label: t('common.totalResources'), value: list.loading ? '—' : list.total },
          { label: t('common.manageScope'), value: t('common.scopePlatform') },
        ]}
      />
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(record) => record.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        unit={t('common.unitRecord')}
        toolbar={
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
            placeholder={t('records.searchPlaceholder')}
            count={t('common.totalCount', { total: list.total, unit: t('common.unitRecord') })}
          />
        }
      />

      <Dialog open={Boolean(detail)} onOpenChange={(open) => !open && setDetail(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{detail?.action}</DialogTitle>
            <DialogDescription>{formatDateTime(detail?.created_at)}</DialogDescription>
          </DialogHeader>
          <dl className="grid gap-3 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground text-xs">{t('common.institution')}</dt>
                <dd>{detail?.institution_id ?? '-'}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground text-xs">{t('users.name')}</dt>
              <dd>{detail?.user || '-'}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground text-xs">{t('records.object')}</dt>
              <dd className="break-all">{detail?.object || '-'}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground text-xs">{t('records.clientIp')}</dt>
              <dd>{detail?.client_ip || '-'}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-muted-foreground text-xs">{t('records.traceId')}</dt>
              <dd className="font-mono text-xs break-all">{detail?.trace_id || '-'}</dd>
            </div>
            {detail?.detail ? (
              <div className="sm:col-span-2">
                <dt className="text-muted-foreground text-xs">{t('records.detail')}</dt>
                <dd className="bg-muted/40 max-h-56 overflow-auto rounded-md p-2 font-mono text-xs break-all">
                  {detail.detail}
                </dd>
              </div>
            ) : null}
          </dl>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDetail(null)}>
              {t('common.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
