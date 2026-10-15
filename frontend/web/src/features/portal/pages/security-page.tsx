import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ShieldCheck, ShieldOff } from 'lucide-react'
import { authApi } from '@/features/auth/auth-api'
import { useAuth } from '@/features/auth/auth-context'
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Badge,
  Skeleton,
} from '@/shared/ui'
import { PortalPage } from '../portal-page'
import { portalApi } from '../portal-api'
import { usePortalAction } from '../hooks/use-portal-action'
import { QrCode } from '@/shared/components/qr-code'
import { CopyButton } from '@/shared/components/copy-button'

export default function PortalSecurityPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { signOut } = useAuth()
  const { pending, run } = usePortalAction()
  const [mfaEnabled, setMfaEnabled] = useState(false)
  const [loading, setLoading] = useState(true)
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [mfaOpen, setMfaOpen] = useState(false)
  const [mfaSecret, setMfaSecret] = useState('')
  const [mfaUrl, setMfaUrl] = useState('')
  const [mfaCode, setMfaCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])

  const load = async () => {
    try {
      const result = await portalApi.me()
      setMfaEnabled(Boolean(result.user?.mfa_enabled))
    } catch {
      setMfaEnabled(false)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const changePassword = async (event: React.FormEvent) => {
    event.preventDefault()
    if (newPassword !== confirmPassword) return
    const ok = await run(
      () => authApi.changePassword({ old_password: oldPassword, new_password: newPassword }),
      'profile.changePasswordSuccess',
    )
    if (!ok) return
    setOldPassword('')
    setNewPassword('')
    setConfirmPassword('')
    await signOut()
    navigate('/login', { replace: true })
  }

  return (
    <PortalPage titleKey="portal.securityTitle" subtitleKey="portal.securitySubtitle">
      <div className="grid gap-5 lg:grid-cols-[1.15fr_0.85fr]">
        <Card>
          <CardHeader>
            <CardTitle>{t('profile.changePassword')}</CardTitle>
            <CardDescription>{t('portal.passwordHint')}</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={changePassword}>
              <div className="space-y-2">
                <Label htmlFor="portal-old-password">{t('profile.oldPassword')}</Label>
                <Input
                  id="portal-old-password"
                  type="password"
                  autoComplete="current-password"
                  value={oldPassword}
                  onChange={(event) => setOldPassword(event.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="portal-new-password">{t('profile.newPassword')}</Label>
                <Input
                  id="portal-new-password"
                  type="password"
                  autoComplete="new-password"
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="portal-confirm-password">{t('portal.confirmPassword')}</Label>
                <Input
                  id="portal-confirm-password"
                  type="password"
                  autoComplete="new-password"
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  required
                  aria-invalid={confirmPassword.length > 0 && newPassword !== confirmPassword}
                />
                {confirmPassword.length > 0 && newPassword !== confirmPassword ? (
                  <p className="text-destructive text-xs">{t('portal.passwordMismatch')}</p>
                ) : null}
              </div>
              <Button
                type="submit"
                disabled={pending || !oldPassword || !newPassword || newPassword !== confirmPassword}
              >
                {t('common.save')}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('profile.mfa')}</CardTitle>
            <CardDescription>{t('portal.mfaHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            {loading ? (
              <Skeleton className="h-10 w-full" />
            ) : (
              <div className="flex items-center justify-between rounded-lg border p-3">
                <span className="text-sm">{t('portal.currentStatus')}</span>
                <Badge variant={mfaEnabled ? 'success' : 'outline'}>
                  {mfaEnabled ? t('profile.mfaEnabled') : t('profile.mfaDisabled')}
                </Badge>
              </div>
            )}
            {mfaEnabled ? (
              <Button
                variant="outline"
                disabled={pending || loading}
                onClick={async () => {
                  const ok = await run(async () => {
                    await authApi.mfaDisable()
                    await load()
                  })
                  if (ok) setRecoveryCodes([])
                }}
              >
                <ShieldOff className="size-4" /> {t('profile.mfaDisable')}
              </Button>
            ) : (
              <Button
                variant="outline"
                disabled={pending || loading}
                onClick={async () => {
                  setRecoveryCodes([])
                  setMfaCode('')
                  const ok = await run(async () => {
                    const result = await authApi.mfaSetup()
                    setMfaSecret(result.secret)
                    setMfaUrl(result.url)
                  })
                  if (ok) setMfaOpen(true)
                }}
              >
                <ShieldCheck className="size-4" /> {t('profile.mfaSetup')}
              </Button>
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog open={mfaOpen} onOpenChange={setMfaOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('profile.mfaSetup')}</DialogTitle>
            <DialogDescription>{t('profile.mfaSetupHint')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {/* 扫码是主路径：直接给二维码，省去手抄密钥 */}
            <div className="space-y-2">
              <p className="text-sm font-medium">{t('auth.mfaScan')}</p>
              <QrCode value={mfaUrl} className="mx-auto" />
              <p className="text-muted-foreground text-center text-xs">{t('auth.mfaScanHint')}</p>
            </div>
            {/* 手动输入兜底 */}
            <div className="space-y-2">
              <p className="text-xs font-medium">{t('auth.mfaManual')}</p>
              <div className="bg-background flex items-center gap-2 rounded-md border px-2 py-1.5">
                <code className="min-w-0 flex-1 truncate font-mono text-xs">{mfaSecret}</code>
                <CopyButton value={mfaSecret} label={t('profile.mfaSecret')} />
              </div>
              <div className="bg-background flex items-center gap-2 rounded-md border px-2 py-1.5">
                <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{mfaUrl}</code>
                <CopyButton value={mfaUrl} label={t('portal.authenticatorUri')} />
              </div>
            </div>
            {recoveryCodes.length === 0 ? (
              <div className="space-y-2">
                <Label htmlFor="portal-mfa-code">{t('auth.mfaCode')}</Label>
                <Input
                  id="portal-mfa-code"
                  value={mfaCode}
                  onChange={(event) => setMfaCode(event.target.value)}
                  inputMode="numeric"
                  maxLength={8}
                  autoComplete="one-time-code"
                  placeholder="000000"
                />
              </div>
            ) : (
              <div className="space-y-2">
                <p className="text-sm font-medium">{t('profile.mfaRecoveryCodes')}</p>
                <div className="grid grid-cols-2 gap-1 rounded-md border p-3 font-mono text-xs">
                  {recoveryCodes.map((code) => (
                    <span key={code}>{code}</span>
                  ))}
                </div>
                <p className="text-muted-foreground text-xs">{t('profile.mfaRecoveryHint')}</p>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setMfaOpen(false)}>
              {t('common.close')}
            </Button>
            {recoveryCodes.length === 0 ? (
              <Button
                disabled={pending || mfaCode.length === 0}
                onClick={async () => {
                  const ok = await run(async () => {
                    const result = await authApi.mfaVerify(mfaCode)
                    setRecoveryCodes(result.recovery_codes ?? [])
                    await load()
                  })
                  if (ok) setMfaCode('')
                }}
              >
                {t('auth.mfaSubmit')}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PortalPage>
  )
}
