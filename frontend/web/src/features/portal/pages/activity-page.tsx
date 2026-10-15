import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Activity } from 'lucide-react'
import {
  Badge,
  Card,
  CardContent,
  EmptyState,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import { StatusPill } from '@/shared/components/status-pill'
import { ListPagination } from '@/widgets/layout/list-pagination'
import { formatDateTime } from '@/shared/lib/format'
import type { PersonalAuditRecord } from '../portal-api'
import { PortalPage } from '../portal-page'
import { portalApi } from '../portal-api'

export default function PortalActivityPage() {
  const { t } = useTranslation()
  const [items, setItems] = useState<PersonalAuditRecord[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const size = 20

  useEffect(() => {
    let active = true
    setLoading(true)
    void portalApi
      .records(page, size)
      .then((result) => {
        if (!active) return
        setItems(result.items ?? [])
        setTotal(result.total ?? 0)
      })
      .catch(() => {
        if (active) {
          setItems([])
          setTotal(0)
        }
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [page])

  return (
    <PortalPage titleKey="portal.activityTitle" subtitleKey="portal.activitySubtitle">
      <Card>
        <CardContent>
          {loading ? (
            <div className="space-y-3">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : items.length === 0 ? (
            <EmptyState
              icon={Activity}
              title={t('portal.noActivity')}
              description={t('portal.noActivityHint')}
            />
          ) : (
            <>
              <div className="border-border/70 shadow-soft overflow-hidden rounded-2xl border">
                <Table>
                  <TableHeader>
                    <TableRow className="hover:bg-transparent">
                      <TableHead>{t('common.createdAt')}</TableHead>
                      <TableHead>{t('portal.activityAction')}</TableHead>
                      <TableHead>{t('portal.activityObject')}</TableHead>
                      <TableHead>{t('portal.activityResult')}</TableHead>
                      <TableHead>{t('portal.activitySource')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {items.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell className="whitespace-nowrap text-xs">
                          {formatDateTime(item.created_at)}
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary">{item.action || '-'}</Badge>
                        </TableCell>
                        <TableCell className="max-w-xs truncate text-xs">
                          {item.object || item.request_uri || '-'}
                        </TableCell>
                        <TableCell>
                          <StatusPill
                            tone={item.status_code >= 500 ? 'danger' : item.status_code >= 400 ? 'warning' : 'success'}
                          >
                            {item.status_code}
                          </StatusPill>
                        </TableCell>
                        <TableCell className="text-xs">{item.client_ip || '-'}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              <ListPagination page={page} size={size} total={total} onPageChange={setPage} unit={t('common.unitRecord')} />
            </>
          )}
        </CardContent>
      </Card>
    </PortalPage>
  )
}
