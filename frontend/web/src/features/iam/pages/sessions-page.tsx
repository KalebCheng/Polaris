import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCw } from 'lucide-react'
import { Badge, Button } from '@/shared/ui'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ListPagination } from '@/widgets/layout/list-pagination'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { StatusPill } from '@/shared/components/status-pill'
import { EmptyState, Skeleton, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui'
import { formatDateTime } from '@/shared/lib/format'
import type { SessionInfo } from '@/shared/api/types'
import { sessionApi } from '../api'
import { useMutation } from '../hooks/use-mutation'
import { usePageHeaderActions } from '../components/page-header-actions'

const PAGE_SIZE = 20

/** 会话管理（平台级，跨组织）：查看全部活动会话并强制下线 */
export default function SessionsPage() {
  const { t } = useTranslation()
  const { pending, run } = useMutation()
  const [loading, setLoading] = useState(true)
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [revoking, setRevoking] = useState<SessionInfo | null>(null)

  const load = async () => {
    setLoading(true)
    try {
      const result = await sessionApi.listAll()
      setSessions(result.items ?? [])
    } catch {
      setSessions([])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const filtered = useMemo(() => {
    const lower = keyword.trim().toLowerCase()
    if (!lower) return sessions
    return sessions.filter((session) =>
      [session.name, session.institution_id, session.ip, session.user_agent, session.email]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(lower)),
    )
  }, [sessions, keyword])

  const paged = filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)
  const orgCount = new Set(sessions.map((session) => session.institution_id)).size

  // 页头操作：刷新按钮的禁用态跟随页面 loading，因此由页面渲染后 portal 到布局页头
  const headerActions = usePageHeaderActions(
    <Button variant="outline" onClick={() => void load()} disabled={loading}>
      <RefreshCw className="size-4" /> {t('common.refresh')}
    </Button>,
  )

  return (
    <>
      {headerActions}
      <SummaryStrip
        items={[
          { label: t('sessions.activeCount'), value: loading ? '—' : sessions.length, tone: 'success' },
          { label: t('sessions.orgCount'), value: loading ? '—' : orgCount },
          { label: t('common.manageScope'), value: t('common.scopePlatform') },
        ]}
      />

      <ListToolbar
        keyword={keyword}
        onKeywordChange={setKeyword}
        placeholder={t('sessions.searchPlaceholder')}
        count={t('common.totalCount', { total: filtered.length, unit: t('common.unitSession') })}
      />

      <div className="bg-card border-border/70 shadow-soft overflow-hidden rounded-2xl border">
        {loading ? (
          <div className="space-y-2 p-3">
            {Array.from({ length: 5 }).map((_, index) => (
              <Skeleton key={index} className="h-10 w-full" />
            ))}
          </div>
        ) : paged.length === 0 ? (
          <EmptyState title={t('common.empty')} description={t('sessions.emptyHint')} />
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead>{t('sessions.user')}</TableHead>
                <TableHead>{t('common.institution')}</TableHead>
                <TableHead>{t('sessions.userAgent')}</TableHead>
                <TableHead>{t('sessions.ip')}</TableHead>
                <TableHead>{t('sessions.expiresAt')}</TableHead>
                <TableHead className="text-right">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {paged.map((session) => (
                <TableRow key={session.id}>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <span className="text-foreground font-medium">{session.name}</span>
                      {session.is_admin ? <Badge variant="info">{t('users.isAdmin')}</Badge> : null}
                      {session.is_current ? <StatusPill tone="success">{t('portal.currentSession')}</StatusPill> : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">{session.institution_id}</Badge>
                  </TableCell>
                  <TableCell className="max-w-[16rem] truncate text-xs">{session.user_agent || '-'}</TableCell>
                  <TableCell className="text-xs">{session.ip || '-'}</TableCell>
                  <TableCell className="text-xs">
                    {formatDateTime(new Date(session.expires_at * 1000).toISOString())}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive"
                      onClick={() => setRevoking(session)}
                    >
                      {t('sessions.revoke')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      {!loading && paged.length > 0 ? (
        <ListPagination page={page} size={PAGE_SIZE} total={filtered.length} onPageChange={setPage} unit={t('common.unitSession')} />
      ) : null}

      <ConfirmDialog
        open={Boolean(revoking)}
        onOpenChange={(open) => !open && setRevoking(null)}
        titleKey="sessions.revoke"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: revoking?.name }}
        confirmLabelKey="sessions.revoke"
        loading={pending}
        onConfirm={async () => {
          if (!revoking) return
          const ok = await run(() => sessionApi.revoke(revoking.id, revoking.name))
          if (ok) {
            setRevoking(null)
            await load()
          }
        }}
      />
    </>
  )
}
