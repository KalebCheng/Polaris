import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ShieldCheck, ShieldOff } from 'lucide-react'
import { authApi, type MFAEnrollResult, type MFAOptions } from '@/shared/api/auth'
import { useAuth } from '@/shared/auth/auth-context'
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
  const [mfaOptions, setMfaOptions] = useState<MFAOptions | null>(null)
  // null = 尚未加载完成；false 表示账号从未设置过密码（如第三方登录自动建号）
  const [hasPassword, setHasPassword] = useState<boolean | null>(null)
  const [loading, setLoading] = useState(true)
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [mfaOpen, setMfaOpen] = useState(false)
  const [mfaCode, setMfaCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [enrollResult, setEnrollResult] = useState<MFAEnrollResult | null>(null)
  const [enrollCompleted, setEnrollCompleted] = useState(false)

  const load = async () => {
    // 两个接口互不依赖：MFA 选项失败不能连带把"尚未设置密码"误判为已有密码。
    const [meResult, optionsResult] = await Promise.allSettled([portalApi.me(), authApi.mfaOptions()])
    const me = meResult.status === 'fulfilled' ? meResult.value : null
    const options = optionsResult.status === 'fulfilled' ? optionsResult.value : null
    // me 失败时按"已有密码"渲染（安全方向）：不会把改密表单降级成免旧密码的首次设密。
    setHasPassword(me ? Boolean(me.user?.has_password) : true)
    setMfaOptions(options)
    setMfaEnabled(Boolean(me?.user?.mfa_enabled) || (options?.bound ?? []).length > 0)
    setLoading(false)
  }

  useEffect(() => {
    void load()
  }, [])

  const changePassword = async (event: React.FormEvent) => {
    event.preventDefault()
    if (hasPassword === null || newPassword !== confirmPassword) return
    const ok = await run(
      () =>
        authApi.changePassword(
          hasPassword
            ? { old_password: oldPassword, new_password: newPassword }
            : { new_password: newPassword },
        ),
      hasPassword ? 'profile.changePasswordSuccess' : 'profile.setPasswordSuccess',
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
            <CardTitle>
              {hasPassword === false ? t('profile.setPassword') : t('profile.changePassword')}
            </CardTitle>
            <CardDescription>
              {hasPassword === false ? t('portal.passwordUnsetHint') : t('portal.passwordHint')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {loading || hasPassword === null ? (
              <Skeleton className="h-44 w-full" />
            ) : (
              <form className="space-y-4" onSubmit={changePassword}>
                {hasPassword ? (
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
                ) : null}
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
                  disabled={
                    pending ||
                    (hasPassword && !oldPassword) ||
                    !newPassword ||
                    newPassword !== confirmPassword
                  }
                >
                  {hasPassword ? t('common.save') : t('profile.setPassword')}
                </Button>
              </form>
            )}
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
            {(mfaOptions?.bound ?? []).length > 0 ? (
              <div className="space-y-2">
                {(mfaOptions?.bound ?? []).map((credential) => (
                  <div
                    key={credential.provider_id}
                    className="flex items-center justify-between rounded-lg border p-3"
                  >
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">
                        {credential.display_name || credential.name}
                      </p>
                      <p className="text-muted-foreground text-xs">{credential.type.toUpperCase()}</p>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={pending || loading}
                      onClick={async () => {
                        const ok = await run(async () => {
                          await authApi.mfaDisable(credential.provider_id)
                          await load()
                        })
                        if (ok) setRecoveryCodes([])
                      }}
                    >
                      <ShieldOff className="size-4" /> {t('profile.mfaDisable')}
                    </Button>
                  </div>
                ))}
              </div>
            ) : null}
            <Button
              variant="outline"
              disabled={pending || loading || (mfaOptions?.providers ?? []).length === 0}
              onClick={async () => {
                setRecoveryCodes([])
                setMfaCode('')
                setEnrollCompleted(false)
                const provider =
                  (mfaOptions?.providers ?? []).find((item) => item.name === 'default-totp') ??
                  (mfaOptions?.providers ?? [])[0]
                if (!provider) return
                const ok = await run(async () => {
                  const result = await authApi.mfaEnroll(provider.id)
                  setEnrollResult(result)
                })
                if (ok) setMfaOpen(true)
              }}
            >
              <ShieldCheck className="size-4" /> {t('profile.mfaSetup')}
            </Button>
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
            {enrollResult?.url ? (
              <>
                <div className="space-y-2">
                  <p className="text-sm font-medium">{t('auth.mfaScan')}</p>
                  <QrCode value={enrollResult.url} className="mx-auto" />
                  <p className="text-muted-foreground text-center text-xs">{t('auth.mfaScanHint')}</p>
                </div>
                <div className="space-y-2">
                  <p className="text-xs font-medium">{t('auth.mfaManual')}</p>
                  <div className="bg-background flex items-center gap-2 rounded-md border px-2 py-1.5">
                    <code className="min-w-0 flex-1 truncate font-mono text-xs">{enrollResult.secret}</code>
                    <CopyButton value={enrollResult.secret ?? ''} label={t('profile.mfaSecret')} />
                  </div>
                  <div className="bg-background flex items-center gap-2 rounded-md border px-2 py-1.5">
                    <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{enrollResult.url}</code>
                    <CopyButton value={enrollResult.url} label={t('portal.authenticatorUri')} />
                  </div>
                </div>
              </>
            ) : enrollResult?.sent ? (
              <p className="text-muted-foreground text-center text-xs">
                {t('auth.mfaSmsSentTo', { phone: enrollResult.phone ?? '' })}
              </p>
            ) : null}
            {!enrollCompleted ? (
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
            {!enrollCompleted ? (
              <Button
                disabled={pending || mfaCode.length === 0 || !enrollResult}
                onClick={async () => {
                  if (!enrollResult) return
                  const ok = await run(async () => {
                    const result = await authApi.mfaEnrollVerify(enrollResult.provider_id, mfaCode)
                    setRecoveryCodes(result.recovery_codes ?? [])
                    setEnrollCompleted(true)
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
