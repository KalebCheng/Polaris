import { useEffect, useState } from 'react'
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
  Skeleton,
} from '@/shared/ui'
import { formatDateTime } from '@/shared/lib/format'
import type { ProvisionHistory } from '@/shared/api/types'
import { provisionHistoryApi } from '../api'

interface ConnectorRunsProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  institution: string
  connector: string
  name: string
}

const actionLabelKeys: Record<string, string> = {
  create: 'provisioning.actionCreate',
  update: 'provisioning.actionUpdate',
  disable: 'provisioning.actionDisable',
  delete: 'provisioning.actionDelete',
  retry: 'provisioning.actionRetry',
}

/**
 * 出方向连接的账号同步记录抽屉。
 *
 * 与入方向的「同步记录」同一交互：记录挂在执行者（连接器）上，
 * 因此这里按 connector 过滤；应用维度的同一份历史在应用详情 › 账号供给里。
 */
export function ConnectorRuns({
  open,
  onOpenChange,
  institution,
  connector,
  name,
}: ConnectorRunsProps) {
  const { t } = useTranslation()
  const [runs, setRuns] = useState<ProvisionHistory[] | null>(null)

  useEffect(() => {
    if (!open || !connector) return
    let cancelled = false
    setRuns(null)
    void (async () => {
      try {
        const result = await provisionHistoryApi.list({
          institution: institution || undefined,
          connector,
          size: 20,
        })
        if (!cancelled) setRuns(result.items ?? [])
      } catch {
        if (!cancelled) setRuns([])
      }
    })()
    return () => {
      cancelled = true
    }
  }, [open, institution, connector])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('connections.provisionRecordsTitle')}</DialogTitle>
          <DialogDescription>{name}</DialogDescription>
        </DialogHeader>
        {runs === null ? (
          <Skeleton className="h-32 w-full" />
        ) : runs.length === 0 ? (
          <p className="text-muted-foreground text-sm">{t('connections.provisionRecordsEmpty')}</p>
        ) : (
          <ul className="divide-border/60 divide-y rounded-xl border text-sm">
            {runs.map((run) => (
              <li key={run.id} className="space-y-1.5 px-3 py-2">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-xs">
                    {formatDateTime(run.created_at)}
                    {run.user ? ` · ${run.user}` : ''}
                  </span>
                  <Badge
                    variant={
                      run.status === 'success'
                        ? 'success'
                        : run.status === 'failed'
                          ? 'destructive'
                          : 'secondary'
                    }
                  >
                    {run.status}
                  </Badge>
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                  <Badge variant="outline">
                    {t(actionLabelKeys[run.action] ?? 'provisioning.action')}
                  </Badge>
                  {run.application ? (
                    <Badge variant="secondary">{run.application}</Badge>
                  ) : null}
                </div>
                {run.message ? (
                  <p className="text-muted-foreground truncate text-[11px]">{run.message}</p>
                ) : null}
              </li>
            ))}
          </ul>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
