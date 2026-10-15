import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Fingerprint, Link2, Unlink } from 'lucide-react'
import { toast } from 'sonner'
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
  Skeleton,
} from '@/shared/ui'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { StatusPill } from '@/shared/components/status-pill'
import { formatDateTime } from '@/shared/lib/format'
import type { ProviderInfo, UserIdentity } from '@/shared/api/types'
import { PortalPage } from '../portal-page'
import { portalApi } from '../portal-api'
import { usePortalAction } from '../hooks/use-portal-action'

export default function PortalIdentitiesPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { pending, run } = usePortalAction()
  const [identities, setIdentities] = useState<UserIdentity[]>([])
  const [providers, setProviders] = useState<ProviderInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [removing, setRemoving] = useState<UserIdentity | null>(null)

  const load = async () => {
    const [linked, available] = await Promise.all([portalApi.identities(), portalApi.identityProviders()])
    setIdentities(linked.items ?? [])
    setProviders(available.providers ?? [])
  }

  useEffect(() => {
    let active = true
    void Promise.all([portalApi.identities(), portalApi.identityProviders()])
      .then(([linked, available]) => {
        if (!active) return
        setIdentities(linked.items ?? [])
        setProviders(available.providers ?? [])
      })
      .catch(() => {
        if (active) toast.error(t('portal.loadFailed'))
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [t])

  useEffect(() => {
    if (searchParams.get('linked') !== '1') return
    toast.success(t('portal.identityLinked'))
    navigate('/portal/identities', { replace: true })
  }, [searchParams, navigate, t])

  const providerNames = new Set(identities.map((identity) => identity.provider))
  const unlinkedProviders = providers.filter((provider) => !providerNames.has(provider.name))

  return (
    <PortalPage titleKey="portal.identitiesTitle" subtitleKey="portal.identitiesSubtitle">
      <Card>
        <CardHeader>
          <CardTitle>{t('portal.linkedIdentities')}</CardTitle>
          <CardDescription>{t('portal.identitiesHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="space-y-3">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-16 w-full" />
            </div>
          ) : identities.length === 0 ? (
            <EmptyState
              icon={Fingerprint}
              title={t('portal.noIdentities')}
              description={t('portal.noIdentitiesHint')}
            />
          ) : (
            <ul className="divide-border/60 divide-y rounded-2xl border border-border/70 shadow-soft">
              {identities.map((identity) => (
                <li key={identity.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
                  <div className="bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg">
                    <Fingerprint className="text-muted-foreground size-5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-medium">{identity.provider}</p>
                      <StatusPill tone="success">{t('portal.linked')}</StatusPill>
                    </div>
                    <p className="text-muted-foreground truncate text-xs">
                      {identity.email || identity.provider_user_id}
                    </p>
                    <p className="text-muted-foreground mt-1 text-xs">
                      {formatDateTime(identity.created_at)}
                    </p>
                  </div>
                  <Button variant="outline" disabled={pending} onClick={() => setRemoving(identity)}>
                    <Unlink className="size-4" /> {t('portal.unlink')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('portal.addIdentity')}</CardTitle>
          <CardDescription>{t('portal.addIdentityHint')}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {unlinkedProviders.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('portal.noProviders')}</p>
          ) : (
            unlinkedProviders.map((provider) => (
              <Button
                key={provider.name}
                variant="outline"
                disabled={pending}
                onClick={async () => {
                  await run(async () => {
                    const result = await portalApi.authorizeIdentityLink(provider.name)
                    window.location.assign(result.authorization_url)
                  })
                }}
              >
                <Link2 className="size-4" /> {provider.display_name || provider.name}
              </Button>
            ))
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="portal.unlinkTitle"
        descriptionKey="portal.unlinkDescription"
        descriptionParams={{ name: removing?.provider }}
        confirmLabelKey="portal.unlink"
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(() => portalApi.unbindIdentity(removing.id), 'portal.identityUnlinked')
          if (ok) {
            setRemoving(null)
            await load()
          }
        }}
      />
    </PortalPage>
  )
}
