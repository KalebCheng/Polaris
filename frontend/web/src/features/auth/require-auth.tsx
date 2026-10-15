import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Spinner } from '@/shared/ui'
import { useAuth } from './auth-context'

/**
 * 登录守卫：未登录时跳转登录页，并用 ?from= 记录原路径（禁止使用 sessionStorage）。
 */
export function RequireAuth() {
  const { authenticated, loading } = useAuth()
  const location = useLocation()
  const { t } = useTranslation()

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center gap-2">
        <Spinner />
        <span className="text-muted-foreground text-sm">{t('common.loading')}</span>
      </div>
    )
  }

  if (!authenticated) {
    const from = `${location.pathname}${location.search}`
    return <Navigate to={`/login?from=${encodeURIComponent(from)}`} replace />
  }

  return <Outlet />
}
