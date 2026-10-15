import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
  Skeleton,
} from '@/shared/ui'
import { StatusPill, type StatusTone } from '@/shared/components/status-pill'
import { UserRoundCog } from 'lucide-react'
import type { AppAccount } from '@/shared/api/types'
import { PortalPage } from '../portal-page'
import { portalApi } from '../portal-api'

const statusTones: Record<string, StatusTone> = {
  active: 'success',
  disabled: 'neutral',
  pending: 'warning',
  failed: 'danger',
}

function statusLabel(status: string, t: (key: string) => string) {
  const keys: Record<string, string> = {
    active: 'common.enabled',
    disabled: 'common.disabled',
    pending: 'portal.accountStatus.pending',
    failed: 'portal.accountStatus.failed',
  }
  return keys[status] ? t(keys[status]) : status
}

export default function PortalAppAccountsPage() {
  const { t } = useTranslation()
  const [items, setItems] = useState<AppAccount[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true
    void portalApi
      .appAccounts()
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

  return (
    <PortalPage titleKey="portal.appAccountsTitle" subtitleKey="portal.appAccountsSubtitle">
      <Card>
        <CardHeader>
          <CardTitle>{t('portal.appAccountsTitle')}</CardTitle>
          <CardDescription>{t('portal.appAccountsHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="space-y-3">
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
            </div>
          ) : items.length === 0 ? (
            <EmptyState
              icon={UserRoundCog}
              title={t('portal.noAppAccounts')}
              description={t('portal.noAppAccountsHint')}
            />
          ) : (
            <ul className="divide-border/60 divide-y rounded-2xl border border-border/70 shadow-soft">
              {items.map((account) => (
                <li key={account.id} className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center">
                  <div className="min-w-0 flex-1">
                    <p className="font-medium">{account.application}</p>
                    <p className="text-muted-foreground text-sm">
                      {t('portal.downstreamAccount')}: {account.related_username || account.username}
                    </p>
                  </div>
                  <StatusPill tone={statusTones[account.status] ?? 'neutral'}>
                    {statusLabel(account.status, t)}
                  </StatusPill>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </PortalPage>
  )
}
