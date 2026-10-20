import { Navigate, Outlet } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Spinner } from '@/shared/ui'
import { useAuth } from './auth-context'

/**
 * 控制台守卫：管理控制台只对"有管理权限"的账号开放——系统管理员，或被授予了
 * 任一管理权限的委派管理员；纯终端用户（无任何管理权限）改去自助门户 /portal。
 *
 * 对齐 Casdoor：控制台面向管理者，普通用户不进管理端（Casdoor 用组织级
 * disableConsole + 权限隐藏隔离；Polaris 已有独立门户，故直接落地到 /portal）。
 */
export function RequireConsoleAccess() {
  const { hasConsoleAccess, loading } = useAuth()
  const { t } = useTranslation()

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center gap-2">
        <Spinner />
        <span className="text-muted-foreground text-sm">{t('common.loading')}</span>
      </div>
    )
  }

  if (!hasConsoleAccess) {
    return <Navigate to="/portal" replace />
  }

  return <Outlet />
}
