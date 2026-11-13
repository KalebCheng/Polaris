import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, Download } from 'lucide-react'
import { Badge, Button, Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { formatRelative } from '@/shared/lib/format'
import { describeError } from '@/shared/lib/errors'
import type { UserImportRecord } from '../api'
import { userImportApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useOnceKey } from '../hooks/use-once-key'
import { ResourceTable, type Column } from '../components/resource-table'

/**
 * 导入历史：导入记录列表（点一行展开该次导入的逐行结果）。
 *
 * 与「导入用户」同一模块：列表回答"谁、什么时候、导了什么、结果如何"，
 * 失败行可下载报告 CSV，修好后回到导入页重导。
 */
export default function UserImportHistoryPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const institutionId = useWorkspaceInstitutionId()
  const navigate = useNavigate()
  const [selected, setSelected] = useState<number | null>(null)

  const list = useListQuery<UserImportRecord>(
    (params) => userImportApi.list({ institution: institution || undefined, ...params }),
    { institution },
  )

  const columns: Column<UserImportRecord>[] = [
    {
      key: 'file',
      headerKey: 'users.import.historyFile',
      render: (row) => (
        <div>
          <div>{row.file_name}</div>
          <div className="text-muted-foreground text-xs">
            {row.operator} · {formatRelative(row.created_at)}
          </div>
        </div>
      ),
    },
    {
      key: 'status',
      headerKey: 'users.import.historyStatus',
      render: (row) => (
        <Badge variant={recordVariant(row.status)}>{t(`users.import.recordStatus.${row.status}`)}</Badge>
      ),
    },
    {
      key: 'summary',
      headerKey: 'users.import.historySummary',
      render: (row) =>
        t('users.import.historySummaryValue', {
          total: row.total_rows,
          created: row.created_count,
          skipped: row.skipped_count,
          failed: row.failed_count,
        }),
    },
    {
      key: 'actions',
      headerKey: 'common.actions',
      render: (row) => (
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={() => setSelected(row.id)}>
            {t('users.import.viewRows')}
          </Button>
          {(row.failed_count > 0 || row.skipped_count > 0) && (
            <a href={userImportApi.reportUrl(row.id)} download>
              <Button size="sm" variant="ghost">
                <Download className="size-4" /> {t('users.import.downloadReport')}
              </Button>
            </a>
          )}
        </div>
      ),
    },
  ]

  return (
    <PageShell
      titleKey="users.import.historyTitle"
      subtitleKey="users.import.historySubtitle"
      actions={
        <>
          <Button variant="ghost" onClick={() => navigate(`/institutions/${institutionId}/users`)}>
            <ArrowLeft className="size-4" /> {t('users.import.backToUsers')}
          </Button>
          <Button variant="outline" onClick={() => navigate(`/institutions/${institutionId}/users/import`)}>
            {t('users.import.goImport')}
          </Button>
        </>
      }
    >
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(row) => row.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
      />
      {selected !== null && (
        <ImportRowsPanel id={selected} institution={institution} onClose={() => setSelected(null)} />
      )}
    </PageShell>
  )
}

function recordVariant(status: UserImportRecord['status']) {
  switch (status) {
    case 'succeeded':
      return 'success' as const
    case 'partial':
      return 'warning' as const
    case 'failed':
      return 'destructive' as const
    default:
      return 'info' as const
  }
}

/** 单次导入的逐行结果（按需加载，StrictMode 下也只请求一次） */
function ImportRowsPanel({
  id,
  institution,
  onClose,
}: {
  id: number
  institution: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<Awaited<ReturnType<typeof userImportApi.detail>>['rows']>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useOnceKey(`import-rows:${id}`, async () => {
    setLoading(true)
    setError('')
    try {
      const result = await userImportApi.detail(id, institution || undefined)
      setRows(result.rows)
    } catch (loadError) {
      setError(describeError(loadError, t))
    } finally {
      setLoading(false)
    }
  })

  const failures = rows.filter((row) => row.status !== 'created')
  return (
    <Card className="mt-4">
      <CardHeader>
        <CardTitle>{t('users.import.rowsTitle', { id })}</CardTitle>
        <CardDescription>
          {loading
            ? t('common.loading')
            : t('users.import.rowsSummary', { total: rows.length, issues: failures.length })}
        </CardDescription>
        <Button size="sm" variant="ghost" className="justify-self-end" onClick={onClose}>
          {t('common.close')}
        </Button>
      </CardHeader>
      <CardContent className="max-h-96 space-y-1 overflow-auto text-sm">
        {error && <div className="text-destructive">{error}</div>}
        {failures.map((row) => (
          <div key={row.row_number} className="flex gap-3 border-b py-1.5 last:border-0">
            <span className="text-muted-foreground w-10 shrink-0 tabular-nums">{row.row_number}</span>
            <span className="shrink-0">{row.user_name}</span>
            <Badge variant={row.status === 'failed' ? 'destructive' : 'warning'}>
              {t(`users.import.status.${row.status}`)}
            </Badge>
            <span className="text-muted-foreground text-xs">
              {[...row.warnings, ...row.errors].map((issue) => issue.message).join(' | ')}
            </span>
          </div>
        ))}
        {!loading && !error && failures.length === 0 && (
          <div className="text-muted-foreground">{t('users.import.allCreated')}</div>
        )}
      </CardContent>
    </Card>
  )
}
