import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Badge,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Button,
  Skeleton,
} from '@/shared/ui'
import { formatDateTime } from '@/shared/lib/format'
import type { SyncerRun } from '@/shared/api/types'
import { syncerApi } from '../api'

interface ConnectionRunsProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  syncerID: number
  name: string
}

/** 同步运行记录抽屉：时间、结果、统计与失败原因（失败明细随 3.3 引擎落地后补全） */
export function ConnectionRuns({ open, onOpenChange, syncerID, name }: ConnectionRunsProps) {
  const { t } = useTranslation()
  const [runs, setRuns] = useState<SyncerRun[] | null>(null)

  useEffect(() => {
    if (!open || syncerID <= 0) return
    let cancelled = false
    setRuns(null)
    void (async () => {
      try {
        const result = await syncerApi.runs(syncerID, { size: 20 })
        if (!cancelled) setRuns(result.items ?? [])
      } catch {
        if (!cancelled) setRuns([])
      }
    })()
    return () => {
      cancelled = true
    }
  }, [open, syncerID])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('connections.runsTitle')}</DialogTitle>
          <DialogDescription>{name}</DialogDescription>
        </DialogHeader>
        {runs === null ? (
          <Skeleton className="h-32 w-full" />
        ) : runs.length === 0 ? (
          <p className="text-muted-foreground text-sm">{t('connections.runsEmpty')}</p>
        ) : (
          <ul className="divide-border/60 divide-y rounded-xl border text-sm">
            {runs.map((run) => (
              <li key={run.id} className="space-y-1.5 px-3 py-2">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-xs">
                    {formatDateTime(run.started_at)}
                    {run.duration_ms ? ` · ${(run.duration_ms / 1000).toFixed(1)}s` : ''}
                    {run.trigger && run.trigger !== 'manual' ? ` · ${run.trigger}` : ''}
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
                {run.counts ? (
                  <p className="text-muted-foreground text-[11px]">
                    {t('connections.runsCounts', {
                      created: run.counts.created,
                      updated: run.counts.updated,
                      skipped: run.counts.skipped,
                      failed: run.counts.failed,
                    })}
                  </p>
                ) : null}
                {run.message ? (
                  <p className="text-muted-foreground truncate text-[11px]">{run.message}</p>
                ) : null}
                {run.errors?.length ? (
                  <ul className="text-destructive space-y-0.5 text-[11px]">
                    {run.errors.slice(0, 5).map((item) => (
                      <li key={item} className="truncate">
                        {item}
                      </li>
                    ))}
                  </ul>
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
