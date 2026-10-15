import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Camera, Mail, MapPin, ShieldCheck } from 'lucide-react'
import { useAuth } from '@/features/auth/auth-context'
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Skeleton,
} from '@/shared/ui'
import type { User } from '@/shared/api/types'
import { PortalPage } from '../portal-page'
import { portalApi } from '../portal-api'
import { usePortalAction } from '../hooks/use-portal-action'

export default function PortalProfilePage() {
  const { t } = useTranslation()
  const { status } = useAuth()
  const { pending, run } = usePortalAction()
  const fileRef = useRef<HTMLInputElement | null>(null)
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  const load = async () => {
    try {
      const result = await portalApi.me()
      setUser(result.user)
    } catch {
      setUser(null)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const displayName = user?.display_name || status?.display_name || status?.name || ''
  const initial = displayName.slice(0, 1).toUpperCase() || 'P'

  return (
    <PortalPage titleKey="portal.profileTitle" subtitleKey="portal.profileSubtitle">
      {loading ? (
        <Skeleton className="h-72 rounded-lg" />
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>{t('profile.accountInfo')}</CardTitle>
            <CardDescription>{t('portal.profileHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
              <Avatar className="size-20">
                {user?.avatar ? <AvatarImage src={user.avatar} alt={displayName} /> : null}
                <AvatarFallback className="text-xl">{initial}</AvatarFallback>
              </Avatar>
              <div className="space-y-2">
                <Input
                  ref={fileRef}
                  type="file"
                  accept="image/png,image/jpeg,image/gif,image/webp"
                  className="hidden"
                  onChange={async (event) => {
                    const file = event.target.files?.[0]
                    if (!file) return
                    await run(async () => {
                      const result = await portalApi.uploadAvatar(file)
                      setUser((current) => (current ? { ...current, avatar: result.avatar } : current))
                    }, 'profile.avatarUpdated')
                    if (fileRef.current) fileRef.current.value = ''
                  }}
                />
                <Button variant="outline" disabled={pending} onClick={() => fileRef.current?.click()}>
                  <Camera className="size-4" /> {t('profile.uploadAvatar')}
                </Button>
                <p className="text-muted-foreground text-xs">{t('profile.avatarHint')}</p>
              </div>
            </div>
            <dl className="grid gap-x-8 gap-y-5 rounded-lg border p-4 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-muted-foreground text-xs">{t('users.name')}</dt>
                <dd className="mt-1 font-medium">{user?.name || status?.name || '-'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground text-xs">{t('users.displayName')}</dt>
                <dd className="mt-1">{user?.display_name || '-'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground flex items-center gap-1 text-xs">
                  <Mail className="size-3" /> {t('users.email')}
                </dt>
                <dd className="mt-1 break-all">{user?.email || '-'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground flex items-center gap-1 text-xs">
                  <MapPin className="size-3" /> {t('portal.institution')}
                </dt>
                  <dd className="mt-1">{user?.institution_id ?? status?.institution_id ?? '-'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground flex items-center gap-1 text-xs">
                  <ShieldCheck className="size-3" /> {t('profile.mfa')}
                </dt>
                <dd className="mt-1">
                  {user?.mfa_enabled ? t('profile.mfaEnabled') : t('profile.mfaDisabled')}
                </dd>
              </div>
            </dl>
          </CardContent>
        </Card>
      )}
    </PortalPage>
  )
}
