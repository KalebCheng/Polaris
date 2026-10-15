import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Button, Spinner } from '@/shared/ui'
import { describeError } from '@/shared/lib/errors'
import { authApi } from './auth-api'
import { AuthShell } from './auth-shell'
import { useAuth } from './auth-context'

/**
 * 魔法链接落地页（/magic?token=…）：校验一次性 token 并建立会话。
 * token 一次性消费，页面刷新或重复打开都会失败——这是预期行为（防重放）。
 */
export default function MagicLinkPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { refresh, isAdmin } = useAuth()
  const [error, setError] = useState('')
  const consumed = useRef(false)

  useEffect(() => {
    // React 严格模式下会执行两次 effect：用 ref 保证 token 只消费一次
    if (consumed.current) return
    consumed.current = true
    const token = searchParams.get('token') ?? ''
    const clientId = searchParams.get('client_id') ?? ''
    if (!token) {
      setError(t('magicLink.missingToken'))
      return
    }
    void (async () => {
      try {
        await authApi.verifyMagicLink({ token, client_id: clientId || undefined })
        await refresh()
        navigate(isAdmin ? '/' : '/portal', { replace: true })
      } catch (err) {
        setError(describeError(err, t))
      }
    })()
  }, [searchParams, refresh, navigate, isAdmin, t])

  return (
    <AuthShell>
      <div className="flex flex-col items-center gap-4 py-8 text-center">
        {error ? (
          <>
            <h1 className="text-lg font-semibold">{t('magicLink.failedTitle')}</h1>
            <p className="text-muted-foreground text-sm">{error}</p>
            <Button onClick={() => navigate('/login', { replace: true })}>{t('magicLink.backToLogin')}</Button>
          </>
        ) : (
          <>
            <Spinner />
            <p className="text-muted-foreground text-sm">{t('magicLink.verifying')}</p>
          </>
        )}
      </div>
    </AuthShell>
  )
}
