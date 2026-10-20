import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { LogOut, Monitor, RotateCw } from 'lucide-react'
import { Button, Card, CardContent, EmptyState, Skeleton } from '@/shared/ui'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { StatusPill } from '@/shared/components/status-pill'
import { formatDateTime } from '@/shared/lib/format'
import { useAuth } from '@/shared/auth/auth-context'
import type { PortalSession } from '../portal-api'
import { PortalPage } from '../portal-page'
import { portalApi } from '../portal-api'
import { usePortalAction } from '../hooks/use-portal-action'

export default function PortalSessionsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { signOut } = useAuth()
  const { pending, run } = usePortalAction()
  const [items, setItems] = useState<PortalSession[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<PortalSession | null>(null)
  const [revokeAll, setRevokeAll] = useState(false)

  const load = async () => {
    const result = await portalApi.sessions()
    setItems(result.items ?? [])
  }

  useEffect(() => {
    let active = true
    void portalApi
      .sessions()
      .then((result) => {
        if (active) setItems(result.items ?? [])
      })
      .catch(() => {
        if (active) setItems([])
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  const signInAgain = async () => {
    await signOut()
    navigate('/login', { replace: true })
  }

  return (
    <PortalPage
      titleKey="portal.sessionsTitle"
      subtitleKey="portal.sessionsSubtitle"
      actions={
        <Button
          variant="outline"
          disabled={pending || loading || items.length === 0}
          onClick={() => setRevokeAll(true)}
        >
          <LogOut className="size-4" /> {t('portal.revokeAllSessions')}
        </Button>
      }
    >
      <Card>
        <CardContent>
          {loading ? (
            <div className="space-y-3">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-16 w-full" />
            </div>
          ) : items.length === 0 ? (
            <EmptyState
              icon={Monitor}
              title={t('portal.noSessions')}
              description={t('portal.noSessionsHint')}
            />
          ) : (
            <ul className="surface divide-border/60 divide-y rounded-2xl">
              {items.map((session) => (
                <li key={session.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
                  <div className="bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg">
                    <Monitor className="text-muted-foreground size-5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-mono text-sm">{session.application || t('portal.browserSession')}</p>
                      {session.is_current ? (
                        <StatusPill tone="success">{t('portal.currentSession')}</StatusPill>
                      ) : null}
                    </div>
                    <p className="text-muted-foreground mt-1 text-xs">
                      {session.ip || t('portal.unknownLocation')} ·{' '}
                      {formatDateTime(new Date(session.created_at * 1000).toISOString())}
                    </p>
                    <p className="text-muted-foreground truncate text-xs">
                      {session.user_agent || t('portal.unknownDevice')}
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {t('portal.expiresAt')}:{' '}
                      {formatDateTime(new Date(session.expires_at * 1000).toISOString())}
                    </p>
                  </div>
                  <Button variant="outline" disabled={pending} onClick={() => setSelected(session)}>
                    <RotateCw className="size-4" /> {t('portal.revokeSession')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={Boolean(selected)}
        onOpenChange={(open) => !open && setSelected(null)}
        titleKey="portal.revokeSession"
        descriptionKey="portal.revokeSessionConfirm"
        descriptionParams={{ name: selected?.application || selected?.ip }}
        confirmLabelKey="portal.revokeSession"
        loading={pending}
        onConfirm={async () => {
          if (!selected) return
          const isCurrent = selected.is_current
          const ok = await run(() => portalApi.revokeSession(selected.id), 'portal.sessionRevoked')
          if (!ok) return
          setSelected(null)
          if (isCurrent) {
            await signInAgain()
            return
          }
          await load()
        }}
      />
      <ConfirmDialog
        open={revokeAll}
        onOpenChange={setRevokeAll}
        titleKey="portal.revokeAllSessions"
        descriptionKey="portal.revokeAllConfirm"
        confirmLabelKey="portal.revokeAllSessions"
        loading={pending}
        onConfirm={async () => {
          const ok = await run(() => portalApi.revokeAllSessions(), 'portal.sessionsRevoked')
          if (!ok) return
          setRevokeAll(false)
          await signInAgain()
        }}
      />
    </PortalPage>
  )
}
