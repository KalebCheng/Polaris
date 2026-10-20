import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Badge,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import { LoadError } from '@/shared/components/load-error'
import { ListPagination } from '@/widgets/layout/list-pagination'
import { formatDateTime } from '@/shared/lib/format'
import { describeError } from '@/shared/lib/errors'
import type { AuditRecord, User } from '@/shared/api/types'
import { userApi } from '../../api'
import { useOnceKey } from '../../hooks/use-once-key'

interface ActivityTabProps {
  user: User
}

const PAGE_SIZE = 20

/** 活动标签：该用户的审计时间线（登录、改密、MFA、授权与状态变更等） */
export function ActivityTab({ user }: ActivityTabProps) {
  const { t } = useTranslation()
  const [items, setItems] = useState<AuditRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const result = await userApi.records(user.id, { page, size: PAGE_SIZE })
      setItems(result.items ?? [])
      setTotal(result.total ?? 0)
    } catch (cause) {
      // 读不到审计记录 ≠ 该用户没有活动记录
      setItems([])
      setTotal(0)
      setError(describeError(cause, t))
    } finally {
      setLoading(false)
    }
  }, [user.id, page, t])

  // key 里带上页码：翻页会重新加载，同一页在 StrictMode 下只加载一次
  useOnceKey(`activity:${user.id}:${page}`, load)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t('users.activityTitle')}</CardTitle>
        <CardDescription>{t('users.activityHint')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {loading ? (
          <Skeleton className="h-40 w-full" />
        ) : error ? (
          <LoadError onRetry={() => void load()} detail={error} />
        ) : items.length === 0 ? (
          <p className="text-muted-foreground text-sm">{t('users.activityEmpty')}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('records.action')}</TableHead>
                <TableHead>{t('records.object')}</TableHead>
                <TableHead>{t('records.clientIp')}</TableHead>
                <TableHead>{t('records.statusCode')}</TableHead>
                <TableHead>{t('records.createdAt')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((record) => (
                <TableRow key={record.id}>
                  <TableCell className="font-mono text-xs">{record.action || '-'}</TableCell>
                  <TableCell className="max-w-[14rem] truncate text-xs">{record.object || '-'}</TableCell>
                  <TableCell className="font-mono text-xs">{record.client_ip || '-'}</TableCell>
                  <TableCell>
                    <Badge variant={record.status_code >= 400 ? 'destructive' : 'success'}>
                      {record.status_code}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs">{formatDateTime(record.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <ListPagination page={page} size={PAGE_SIZE} total={total} onPageChange={setPage} />
      </CardContent>
    </Card>
  )
}
