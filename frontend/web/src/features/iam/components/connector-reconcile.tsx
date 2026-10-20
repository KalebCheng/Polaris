import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/shared/ui/toast'
import { RefreshCw } from 'lucide-react'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Skeleton,
} from '@/shared/ui'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { describeError } from '@/shared/lib/errors'
import { connectorApi, type ReconcileReport } from '../api'

interface ConnectorReconcileProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  connectorID: number
  applicationID: number
  name: string
}

/**
 * 出方向对齐检查：核对"本地已有应用账号"在下游是否真实存在。
 *
 * 语义（与后端一致，安全优先）：
 *   - 只做保守方向——确认缺失的才补建号；**不做"下游有、本地没有就删除"**（那需要可靠的全量枚举）；
 *   - 「无法判定」单独计数并展示原因（如下游权限不足），**绝不当作缺失**，避免重复建号。
 */
export function ConnectorReconcile({
  open,
  onOpenChange,
  connectorID,
  applicationID,
  name,
}: ConnectorReconcileProps) {
  const { t } = useTranslation()
  const [report, setReport] = useState<ReconcileReport | null>(null)
  const [busy, setBusy] = useState(false)

  const run = useCallback(
    async (dryRun: boolean) => {
      setBusy(true)
      try {
        const result = await connectorApi.reconcile(connectorID, applicationID, dryRun)
        setReport(result)
        if (!dryRun) {
          toast.success(t('connections.reconcileQueued', { count: result.queued }))
        }
      } catch (error) {
        toast.error(describeError(error, t))
        setReport(null)
      } finally {
        setBusy(false)
      }
    },
    [applicationID, connectorID, t],
  )

  useEffect(() => {
    if (!open || connectorID <= 0) return
    setReport(null)
    void run(true)
  }, [open, connectorID, run])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('connections.reconcile')}</DialogTitle>
          <DialogDescription>{t('connections.reconcileHint', { name })}</DialogDescription>
        </DialogHeader>

        {report === null ? (
          <Skeleton className="h-32 w-full" />
        ) : (
          <div className="space-y-4">
            <SummaryStrip
              variant="inline"
              items={[
                { label: t('connections.reconcileTotal'), value: report.total },
                { label: t('connections.reconcileOk'), value: report.ok, tone: 'success' },
                {
                  label: t('connections.reconcileMissing'),
                  value: report.missing,
                  tone: report.missing > 0 ? 'warning' : 'default',
                },
                {
                  label: t('connections.reconcileUnknown'),
                  value: report.unknown,
                  tone: report.unknown > 0 ? 'danger' : 'default',
                },
              ]}
            />
            {report.items.length ? (
              <ul className="divide-border/60 max-h-72 divide-y overflow-y-auto rounded-xl border text-sm">
                {report.items.map((item) => (
                  <li key={item.user} className="flex items-start justify-between gap-3 px-3 py-2">
                    <div className="min-w-0">
                      <p className="truncate font-mono text-[12px]">{item.user}</p>
                      {item.detail ? (
                        <p className="text-muted-foreground truncate text-[11px]">{item.detail}</p>
                      ) : null}
                    </div>
                    <Badge
                      variant={
                        item.status === 'ok' ? 'success' : item.status === 'missing' ? 'warning' : 'secondary'
                      }
                    >
                      {t(`connections.reconcileState_${item.status}`)}
                    </Badge>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-muted-foreground text-sm">{t('connections.reconcileEmpty')}</p>
            )}
            {report.truncated ? (
              <p className="text-muted-foreground text-[11px]">{t('connections.reconcileTruncated')}</p>
            ) : null}
            {report.unknown > 0 ? (
              <p className="text-muted-foreground text-[11px]">{t('connections.reconcileUnknownHint')}</p>
            ) : null}
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => void run(true)} disabled={busy}>
            <RefreshCw className="size-4" /> {t('connections.reconcileRerun')}
          </Button>
          {report && report.missing > 0 ? (
            <Button onClick={() => void run(false)} disabled={busy}>
              {t('connections.reconcileProvision', { count: report.missing })}
            </Button>
          ) : null}
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
