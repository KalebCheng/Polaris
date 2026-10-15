import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound, MonitorSmartphone, RotateCcw, ShieldCheck, ShieldOff, Unlock } from 'lucide-react'
import {
  Badge,
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
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { LoadError } from '@/shared/components/load-error'
import { describeError } from '@/shared/lib/errors'
import { formatDateTime, formatRelative } from '@/shared/lib/format'
import type { LoginLockout, SessionInfo, User, UserSecurityStatus } from '@/shared/api/types'
import { sessionApi, userApi } from '../../api'
import { useWorkspaceInstitution } from '../../institution-context'
import { useMutation } from '../../hooks/use-mutation'
import { useOnceKey } from '../../hooks/use-once-key'
import { Field } from '../field'

interface SecurityTabProps {
  user: User
  onChanged: () => Promise<void> | void
}

/**
 * 安全与认证标签：密码状态、MFA、登录失败锁定、活动会话。
 *
 * 这一屏回答运维每天真正要问的三个问题：这个人的凭证是否正常、会不会被第二因子挡住、
 * 是否正在被登录限制/Kick 下线，以及能不能立刻处置。
 */
export function SecurityTab({ user, onChanged }: SecurityTabProps) {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const { pending, run } = useMutation()
  const [status, setStatus] = useState<UserSecurityStatus | null>(null)
  const [lockouts, setLockouts] = useState<LoginLockout[]>([])
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [loading, setLoading] = useState(true)
  // 两个独立请求各自的错误态：任一失败都不得退化成"空 = 没问题"
  const [securityError, setSecurityError] = useState('')
  const [sessionsError, setSessionsError] = useState('')

  const [resetPasswordOpen, setResetPasswordOpen] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [resetMfaOpen, setResetMfaOpen] = useState(false)
  const [unlockOpen, setUnlockOpen] = useState(false)
  const [revokeTarget, setRevokeTarget] = useState<SessionInfo | null>(null)
  const [revokeAllOpen, setRevokeAllOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setSecurityError('')
    setSessionsError('')
    // 用 allSettled：一个失败不影响另一个的展示，但失败的一方必须显式报错
    const [securityResult, sessionResult] = await Promise.allSettled([
      userApi.security(user.id),
      // 会话索引按「组织 + 账号」存储，跨组织查看时必须带上工作区组织（否则会查到空）
      sessionApi.listByUser(user.name, institution || undefined),
    ])
    if (securityResult.status === 'fulfilled') {
      setStatus(securityResult.value.password)
      setLockouts(securityResult.value.lockouts ?? [])
    } else {
      setSecurityError(describeError(securityResult.reason, t))
    }
    if (sessionResult.status === 'fulfilled') {
      setSessions(sessionResult.value.items ?? [])
    } else {
      setSessionsError(describeError(sessionResult.reason, t))
    }
    setLoading(false)
  }, [user.id, user.name, institution, t])

  // 首次激活加载一次（StrictMode 下不会翻倍；refresh 走显式调用）
  useOnceKey(`security:${user.id}`, load)

  const refresh = async () => {
    await load()
    await onChanged()
  }

  const blocked = lockouts.some((entry) => entry.blocked)

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {securityError && !loading ? (
        <div className="lg:col-span-2">
          <LoadError onRetry={() => void load()} detail={securityError} />
        </div>
      ) : (
        <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <KeyRound className="size-4" /> {t('users.sectionPassword')}
          </CardTitle>
          <CardDescription>{t('users.sectionPasswordHint')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : (
            <dl className="space-y-3 text-sm">
              <div className="flex items-center justify-between gap-3">
                <dt className="text-muted-foreground">{t('users.passwordState')}</dt>
                <dd>
                  {status?.has_password ? (
                    <Badge variant="success">{t('users.hasPassword')}</Badge>
                  ) : (
                    <Badge variant="outline">{t('users.noPassword')}</Badge>
                  )}
                </dd>
              </div>
              <div className="flex items-center justify-between gap-3">
                <dt className="text-muted-foreground">{t('users.passwordUpdatedAt')}</dt>
                <dd>{status?.password_updated_at ? formatDateTime(status.password_updated_at) : '-'}</dd>
              </div>
              <div className="flex items-center justify-between gap-3">
                <dt className="text-muted-foreground">{t('users.passwordExpireAt')}</dt>
                <dd>{status?.password_expire_at ? formatDateTime(status.password_expire_at) : t('users.passwordNoExpiry')}</dd>
              </div>
            </dl>
          )}
          <Button variant="outline" size="sm" onClick={() => setResetPasswordOpen(true)}>
            <RotateCcw className="size-4" /> {t('users.resetPassword')}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="size-4" /> {t('users.sectionMfa')}
          </CardTitle>
          <CardDescription>{t('users.sectionMfaHint')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : (
            <dl className="space-y-3 text-sm">
              <div className="flex items-center justify-between gap-3">
                <dt className="text-muted-foreground">{t('users.mfaState')}</dt>
                <dd>
                  {status?.mfa_enabled ? (
                    <Badge variant="secondary">{(status.mfa_type || 'totp').toUpperCase()}</Badge>
                  ) : (
                    <Badge variant="outline">{t('users.mfaNotBound')}</Badge>
                  )}
                </dd>
              </div>
              <div className="flex items-center justify-between gap-3">
                <dt className="text-muted-foreground">{t('users.mfaRecoveryLeft')}</dt>
                <dd>{status?.mfa_recovery_codes_left ?? 0}</dd>
              </div>
            </dl>
          )}
          <Button variant="outline" size="sm" onClick={() => setResetMfaOpen(true)} disabled={!status?.mfa_enabled}>
            <ShieldOff className="size-4" /> {t('users.resetMfa')}
          </Button>
        </CardContent>
      </Card>

      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Unlock className="size-4" /> {t('users.sectionLockout')}
          </CardTitle>
          <CardDescription>{t('users.sectionLockoutHint')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : lockouts.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('users.lockoutEmpty')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('users.lockoutApplication')}</TableHead>
                  <TableHead>{t('users.lockoutIp')}</TableHead>
                  <TableHead>{t('users.lockoutFailures')}</TableHead>
                  <TableHead>{t('users.lockoutRemaining')}</TableHead>
                  <TableHead>{t('users.status')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {lockouts.map((entry) => (
                  <TableRow key={`${entry.application}-${entry.ip}`}>
                    <TableCell>{entry.application || t('users.lockoutNoApplication')}</TableCell>
                    <TableCell className="font-mono text-xs">{entry.ip}</TableCell>
                    <TableCell>{entry.failures}</TableCell>
                    <TableCell>{Math.max(0, Math.round(entry.frozen_seconds_left / 60))} min</TableCell>
                    <TableCell>
                      {entry.blocked ? (
                        <Badge variant="destructive">{t('users.lockoutBlocked')}</Badge>
                      ) : (
                        <Badge variant="outline">{t('users.lockoutCounting')}</Badge>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          <Button variant="outline" size="sm" onClick={() => setUnlockOpen(true)} disabled={lockouts.length === 0}>
            <Unlock className="size-4" /> {t('users.unlock')}
          </Button>
          {blocked ? <p className="text-destructive text-xs">{t('users.lockoutBlockedHint')}</p> : null}
        </CardContent>
      </Card>

        </>
      )}

      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <MonitorSmartphone className="size-4" /> {t('users.sectionSessions')}
          </CardTitle>
          <CardDescription>{t('users.sectionSessionsHint')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : sessionsError ? (
            <LoadError onRetry={() => void load()} detail={sessionsError} />
          ) : sessions.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('users.sessionsEmpty')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('sessions.ip')}</TableHead>
                  <TableHead>{t('sessions.userAgent')}</TableHead>
                  <TableHead>{t('sessions.createdAt')}</TableHead>
                  <TableHead>{t('sessions.expiresAt')}</TableHead>
                  <TableHead className="text-right">{t('common.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sessions.map((session) => (
                  <TableRow key={session.id}>
                    <TableCell className="font-mono text-xs">{session.ip || '-'}</TableCell>
                    <TableCell className="max-w-[18rem] truncate text-xs">{session.user_agent || '-'}</TableCell>
                    <TableCell className="text-xs">{formatRelative(new Date(session.created_at * 1000).toISOString())}</TableCell>
                    <TableCell className="text-xs">{formatDateTime(new Date(session.expires_at * 1000).toISOString())}</TableCell>
                    <TableCell className="text-right">
                      <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setRevokeTarget(session)}>
                        {t('sessions.revoke')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          <Button
            variant="outline"
            size="sm"
            className="text-destructive"
            onClick={() => setRevokeAllOpen(true)}
            disabled={sessions.length === 0}
          >
            {t('users.revokeAllSessions')}
          </Button>
        </CardContent>
      </Card>

      <Dialog open={resetPasswordOpen} onOpenChange={setResetPasswordOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('users.resetPasswordTitle')}</DialogTitle>
            <DialogDescription>{user.name}</DialogDescription>
          </DialogHeader>
          <Field id="user-new-password" labelKey="users.newPassword" hint={t('users.resetPasswordHint')}>
            <Input
              id="user-new-password"
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
            />
          </Field>
          <DialogFooter>
            <Button variant="outline" onClick={() => setResetPasswordOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              disabled={pending || !newPassword}
              onClick={async () => {
                const ok = await run(async () => {
                  await userApi.resetPassword(user.id, newPassword)
                })
                if (ok) {
                  setNewPassword('')
                  setResetPasswordOpen(false)
                  await refresh()
                }
              }}
            >
              {t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={resetMfaOpen}
        onOpenChange={setResetMfaOpen}
        titleKey="users.resetMfaConfirmTitle"
        descriptionKey="users.resetMfaConfirmBody"
        descriptionParams={{ name: user.name }}
        loading={pending}
        onConfirm={async () => {
          const ok = await run(() => userApi.resetMfa(user.id), 'users.resetMfaDone')
          if (ok) {
            setResetMfaOpen(false)
            await refresh()
          }
        }}
      />

      <ConfirmDialog
        open={unlockOpen}
        onOpenChange={setUnlockOpen}
        titleKey="users.unlockConfirmTitle"
        descriptionKey="users.unlockConfirmBody"
        descriptionParams={{ name: user.name }}
        loading={pending}
        onConfirm={async () => {
          const ok = await run(() => userApi.unlock(user.id), 'users.unlockDone')
          if (ok) {
            setUnlockOpen(false)
            await refresh()
          }
        }}
      />

      <ConfirmDialog
        open={Boolean(revokeTarget)}
        onOpenChange={(open) => !open && setRevokeTarget(null)}
        titleKey="users.revokeSessionConfirmTitle"
        descriptionKey="users.revokeSessionConfirmBody"
        loading={pending}
        onConfirm={async () => {
          if (!revokeTarget) return
          const ok = await run(() => sessionApi.revoke(revokeTarget.id, user.name))
          if (ok) {
            setRevokeTarget(null)
            await refresh()
          }
        }}
      />

      <ConfirmDialog
        open={revokeAllOpen}
        onOpenChange={setRevokeAllOpen}
        titleKey="users.revokeAllConfirmTitle"
        descriptionKey="users.revokeAllConfirmBody"
        descriptionParams={{ name: user.name }}
        loading={pending}
        onConfirm={async () => {
          const ok = await run(() => sessionApi.revokeAll(user.name))
          if (ok) {
            setRevokeAllOpen(false)
            await refresh()
          }
        }}
      />
    </div>
  )
}
