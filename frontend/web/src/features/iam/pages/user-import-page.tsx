import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, Download, FileUp, Upload } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Switch,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { toast } from '@/shared/ui/toast'
import { describeError } from '@/shared/lib/errors'
import type { UserImportReport, UserImportRow } from '../api'
import { userImportApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { usePageHeaderActions } from '../components/page-header-actions'

/** 阶段：待选择 → 上传中 → 服务端处理中 → 结果 */
type Phase = 'idle' | 'uploading' | 'processing' | 'done'

function downloadBlob(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = fileName
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function rowErrors(row: UserImportRow) {
  return [...row.warnings, ...row.errors]
}

function statusVariant(status: UserImportRow['status']) {
  if (status === 'created') return 'success' as const
  if (status === 'skipped') return 'warning' as const
  return 'destructive' as const
}

/**
 * 导入用户（单阶段）：选文件 + 选项 → 一次请求完成解析 / 校验 / 分批创建 → 返回逐行报告。
 *
 * 导入只做**新建**：已存在的账号名 / 邮箱跳过，绝不改写存量用户；失败行可下载 CSV 修好后重导。
 */
export default function UserImportPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const institutionId = useWorkspaceInstitutionId()
  const institution = useWorkspaceInstitution()
  const fileInput = useRef<HTMLInputElement>(null)

  const [file, setFile] = useState<File | null>(null)
  const [autoCreateDepartments, setAutoCreateDepartments] = useState(false)
  const [phase, setPhase] = useState<Phase>('idle')
  const [uploadRatio, setUploadRatio] = useState(0)
  const [report, setReport] = useState<UserImportReport | null>(null)

  const backToUsers = () => navigate(`/institutions/${institutionId}/users`)

  const headerActions = usePageHeaderActions(
    <Button variant="ghost" onClick={backToUsers}>
      <ArrowLeft className="size-4" /> {t('users.import.backToUsers')}
    </Button>,
  )

  const downloadTemplate = async () => {
    try {
      const lang = t('users.import.templateLang') === 'en' ? 'en' : 'zh'
      const blob = await userImportApi.template(lang)
      downloadBlob(blob, `polaris-user-import-template-${lang}.xlsx`)
    } catch (error) {
      toast.error(describeError(error, t))
    }
  }

  const reset = () => {
    setFile(null)
    setPhase('idle')
    setUploadRatio(0)
    setReport(null)
    if (fileInput.current) fileInput.current.value = ''
  }

  const upload = async () => {
    if (!file) return
    setPhase('uploading')
    setUploadRatio(0)
    try {
      const result = await userImportApi.upload(file, {
        institution: institution || undefined,
        autoCreateDepartments,
        onUploadProgress: (ratio) => {
          setUploadRatio(ratio)
          if (ratio >= 1) setPhase('processing')
        },
      })
      setReport(result)
      setPhase('done')
    } catch (error) {
      toast.error(describeError(error, t))
      setPhase('idle')
    }
  }

  const openHistory = () => navigate(`/institutions/${institutionId}/users/imports`)

  return (
    <PageShell
      titleKey="users.import.title"
      subtitleKey="users.import.subtitle"
      actions={
        <>
          {headerActions}
          <Button variant="outline" onClick={openHistory}>
            {t('users.import.historyLink')}
          </Button>
        </>
      }
    >
      {!report ? (
        <Card className="max-w-3xl">
          <CardHeader>
            <CardTitle>{t('users.import.stepUpload')}</CardTitle>
            <CardDescription>{t('users.import.uploadHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" onClick={downloadTemplate}>
                <Download className="size-4" /> {t('users.import.downloadTemplate')}
              </Button>
              <Button
                variant="outline"
                onClick={() => fileInput.current?.click()}
                disabled={phase !== 'idle'}
              >
                <FileUp className="size-4" /> {t('users.import.chooseFile')}
              </Button>
              <span className="text-muted-foreground text-sm">
                {file ? file.name : t('users.import.noFile')}
              </span>
              <input
                ref={fileInput}
                type="file"
                accept=".xlsx,.csv"
                className="hidden"
                onChange={(event) => {
                  setFile(event.target.files?.[0] ?? null)
                  setPhase('idle')
                  setReport(null)
                }}
              />
            </div>
            <label className="flex items-center gap-3 text-sm">
              <Switch checked={autoCreateDepartments} onCheckedChange={setAutoCreateDepartments} />
              <span>
                {t('users.import.autoCreateDepartments')}
                <span className="text-muted-foreground block text-xs">
                  {t('users.import.autoCreateDepartmentsHint')}
                </span>
              </span>
            </label>

            {phase === 'uploading' && (
              <div className="space-y-1" aria-live="polite">
                <div className="text-muted-foreground text-xs">
                  {t('users.import.uploading', { percent: Math.round(uploadRatio * 100) })}
                </div>
                <div className="bg-secondary h-2 overflow-hidden rounded-full">
                  <div
                    className="bg-primary h-full transition-[width]"
                    style={{ width: `${Math.round(uploadRatio * 100)}%` }}
                  />
                </div>
              </div>
            )}
            {phase === 'processing' && (
              <div className="text-muted-foreground text-xs" aria-live="polite">
                {t('users.import.processingUnknown')}
              </div>
            )}

            <div className="flex items-center gap-2">
              <Button onClick={upload} disabled={!file || phase === 'uploading' || phase === 'processing'}>
                <Upload className="size-4" /> {t('users.import.start')}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : (
        <ImportResult report={report} onReset={reset} onHistory={openHistory} />
      )}
    </PageShell>
  )
}

function ImportResult({
  report,
  onReset,
  onHistory,
}: {
  report: UserImportReport
  onReset: () => void
  onHistory: () => void
}) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState<'all' | 'created' | 'skipped' | 'failed'>('all')
  const rows = report.rows.filter((row) => filter === 'all' || row.status === filter)
  const { summary } = report

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('users.import.resultTitle')}</CardTitle>
          <CardDescription>
            {t('users.import.resultSummary', {
              total: summary.total,
              created: summary.created,
              skipped: summary.skipped,
              failed: summary.failed,
            })}
          </CardDescription>
          <CardAction className="flex gap-2">
            <Button variant="outline" onClick={onHistory}>
              {t('users.import.viewHistory')}
            </Button>
            <Button onClick={onReset}>{t('users.import.importAnother')}</Button>
          </CardAction>
        </CardHeader>
        <CardContent className="space-y-3">
          {summary.created_departments.length > 0 && (
            <div className="text-sm">
              <span className="text-muted-foreground">{t('users.import.createdDepartments')}: </span>
              {summary.created_departments.join(' · ')}
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            {(['all', 'created', 'skipped', 'failed'] as const).map((key) => {
              const count =
                key === 'all'
                  ? summary.total
                  : key === 'created'
                    ? summary.created
                    : key === 'skipped'
                      ? summary.skipped
                      : summary.failed
              return (
                <Button
                  key={key}
                  size="sm"
                  variant={filter === key ? 'default' : 'outline'}
                  onClick={() => setFilter(key)}
                >
                  {t(`users.import.filter.${key}`)} ({count})
                </Button>
              )
            })}
            <a href={userImportApi.reportUrl(report.import_id)} download>
              <Button size="sm" variant="outline">
                <Download className="size-4" /> {t('users.import.downloadReport')}
              </Button>
            </a>
          </div>
          <div className="max-h-[26rem] overflow-auto rounded-xl border">
            <table className="w-full text-sm">
              <thead className="bg-muted/50 text-muted-foreground sticky top-0 text-left text-xs">
                <tr>
                  <th className="px-3 py-2">{t('users.import.rowNumber')}</th>
                  <th className="px-3 py-2">{t('users.import.rowUser')}</th>
                  <th className="px-3 py-2">{t('users.import.rowStatus')}</th>
                  <th className="px-3 py-2">{t('users.import.rowReason')}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.row} className="border-t">
                    <td className="text-muted-foreground px-3 py-2 tabular-nums">{row.row}</td>
                    <td className="px-3 py-2">
                      <div>{row.name}</div>
                      {row.email && <div className="text-muted-foreground text-xs">{row.email}</div>}
                    </td>
                    <td className="px-3 py-2">
                      <Badge variant={statusVariant(row.status)}>
                        {t(`users.import.status.${row.status}`)}
                      </Badge>
                    </td>
                    <td className="px-3 py-2">
                      {rowErrors(row).map((issue, index) => (
                        <div key={index} className="text-xs">
                          {issue.column && (
                            <span className="text-muted-foreground">
                              {t(`users.import.columns.${issue.column}`, { defaultValue: issue.column })}
                              :{' '}
                            </span>
                          )}
                          {issue.message}
                        </div>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
