import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { AlertTriangle } from 'lucide-react'
import { Button, Spinner } from '@/shared/ui'
import { describeError } from '@/shared/lib/errors'
import { authApi } from './auth-api'
import { AuthShell } from './auth-shell'
import { useAuth } from './auth-context'

/**
 * OAuth / OIDC 回调中间页（路由挂在前端命名空间 /sso/callback，与后端 /oauth/* 协议端点分开）。
 * 由身份提供方携带 code + state 回跳，这里完成凭据交换并回跳原页面（from 由 state 在服务端暂存）。
 * 与登录页共用同一套极简舞台，保持品牌一致。
 */
export default function OAuthCallbackPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { refresh } = useAuth()
  const [error, setError] = useState<string | null>(null)
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true

    const code = searchParams.get('code')
    const state = searchParams.get('state')
    const provider = window.location.pathname.split('/').filter(Boolean).pop() ?? ''

    if (!code || !state) {
      setError(t('auth.callbackFailedHint'))
      return
    }

    void (async () => {
      try {
        const result = await authApi.oauthCallback(provider, code, state)
        // 反代把 /api 指到静态站等情况下，响应体不是 JSON，request 会解出 undefined：
        // 兜底成"回调失败"，避免随后读取 result 抛出 TypeError 这种看不懂的提示
        if (!result) {
          setError(t('auth.callbackFailedHint'))
          return
        }
        if (result.identity_linked) {
          navigate('/portal/identities?linked=1', { replace: true })
          return
        }
        await refresh()
        const target = result.session?.from
        navigate(target && target.startsWith('/') && !target.startsWith('//') ? target : '/', {
          replace: true,
        })
      } catch (err) {
        setError(describeError(err, t))
      }
    })()
  }, [searchParams, refresh, navigate, t])

  return (
    <AuthShell>
      <div className="flex flex-col gap-6 text-center">
        <div className="mx-auto grid size-11 place-items-center rounded-full border">
          {error ? (
            <AlertTriangle className="text-destructive size-5" aria-hidden="true" />
          ) : (
            <Spinner className="size-5" />
          )}
        </div>

        <div className="space-y-2">
          <h1 className="text-xl font-semibold tracking-tight">
            {error ? t('auth.callbackFailed') : t('auth.callbackTitle')}
          </h1>
          <p className="text-muted-foreground text-sm leading-relaxed">
            {error ? t('auth.callbackFailedHint') : t('auth.callbackHint')}
          </p>
        </div>

        {error ? (
          <>
            <p className="text-muted-foreground text-xs">{error}</p>
            <Button asChild variant="outline" className="h-10 w-full">
              <Link to="/login">{t('auth.backToLogin')}</Link>
            </Button>
          </>
        ) : null}
      </div>
    </AuthShell>
  )
}
