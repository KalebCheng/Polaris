import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowUpRight, AppWindow } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
  Skeleton,
} from '@/shared/ui'
import { Avatar, AvatarFallback, AvatarImage } from '@/shared/ui'
import { PortalPage } from '../portal-page'
import { buildAuthorizeURL, portalApi, safeHomepageURL, type PortalApplication } from '../portal-api'

export default function PortalAppsPage() {
  const { t } = useTranslation()
  const [items, setItems] = useState<PortalApplication[]>([])
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let active = true
    void portalApi
      .applications()
      .then((result) => {
        if (active) setItems(result.items ?? [])
      })
      .catch(() => {
        if (active) setFailed(true)
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  return (
    <PortalPage titleKey="portal.appsTitle" subtitleKey="portal.appsSubtitle">
      {loading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-44 rounded-lg" />
          ))}
        </div>
      ) : failed ? (
        <EmptyState title={t('portal.loadFailed')} description={t('portal.tryAgain')} />
      ) : items.length === 0 ? (
        <EmptyState icon={AppWindow} title={t('portal.noApps')} description={t('portal.noAppsHint')} />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {items.map((app) => {
            // 优先一键 SSO（发起 OIDC 授权），否则回退到应用主页链接
            const ssoURL = buildAuthorizeURL(app)
            const href = ssoURL ?? safeHomepageURL(app.homepage_url)
            return (
              <Card key={app.name} className="transition-shadow hover:shadow-md">
                <CardHeader className="flex items-start gap-3">
                  <Avatar className="size-12 rounded-lg">
                    {app.logo ? <AvatarImage src={app.logo} alt="" className="object-cover" /> : null}
                    <AvatarFallback className="rounded-lg bg-primary/10 text-primary">
                      <AppWindow className="size-5" />
                    </AvatarFallback>
                  </Avatar>
                  <div className="min-w-0 flex-1">
                    <CardTitle className="truncate text-base">{app.display_name}</CardTitle>
                    <CardDescription className="mt-1 truncate">{app.name}</CardDescription>
                  </div>
                  <Badge variant="secondary">{t('portal.available')}</Badge>
                </CardHeader>
                <CardContent className="flex h-full flex-col gap-4">
                  <p className="text-muted-foreground line-clamp-2 min-h-10 text-sm">
                    {app.description || t('portal.appDescriptionFallback')}
                  </p>
                  {href ? (
                    <Button asChild className="mt-auto w-full">
                      <a href={href} target="_blank" rel="noreferrer">
                        {t('portal.openApp')} <ArrowUpRight className="size-4" />
                      </a>
                    </Button>
                  ) : (
                    <Button disabled className="mt-auto w-full">
                      {t('portal.appUnavailable')}
                    </Button>
                  )}
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
    </PortalPage>
  )
}
