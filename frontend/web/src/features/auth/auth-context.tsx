import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { authApi } from './auth-api'
import type { AuthStatus } from '@/shared/api/types'

interface AuthContextValue {
  status: AuthStatus | null
  loading: boolean
  authenticated: boolean
  isAdmin: boolean
  /** 是否可进入管理控制台：管理员，或被授予了任一管理权限的委派管理员 */
  hasConsoleAccess: boolean
  refresh: () => Promise<void>
  signOut: () => Promise<void>
  /** 是否具备某资源/动作权限（管理员直通） */
  can: (resource: string, action: string) => boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [loading, setLoading] = useState(true)

  // 引导期只查一次会话状态：StrictMode 的 effect 双调用（开发环境）与任何重复挂载都不该再打一次网络。
  // 显式 refresh()（登录/登出/回调后）不受此守卫影响。
  const bootstrappedRef = useRef(false)

  const refresh = useCallback(async () => {
    try {
      const next = await authApi.status()
      setStatus(next)
    } catch {
      setStatus({ authenticated: false, institution_id: 0 })
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (bootstrappedRef.current) return
    bootstrappedRef.current = true
    void refresh()
  }, [refresh])

  const signOut = useCallback(async () => {
    try {
      await authApi.logout()
    } catch {
      // 服务端会话可能已被撤销；无论远端结果如何都清除本地认证状态。
    } finally {
      setStatus({ authenticated: false, institution_id: 0 })
    }
  }, [])

  const value = useMemo<AuthContextValue>(() => {
    const isAdmin = Boolean(status?.is_admin)
    const can = (resource: string, action: string) => {
      if (!status?.authenticated) return false
      if (isAdmin) return true
      const rules = status.permissions ?? []
      let allowed = false
      for (const rule of rules) {
        if (rule.resource !== '*' && rule.resource !== resource) continue
        if (rule.action !== '*' && rule.action !== action) continue
        if (rule.effect === 'Deny') return false
        allowed = true
      }
      return allowed
    }
    // 可进控制台 = 管理员，或持有任一资源读权限（委派管理员）；纯终端用户只能进门户
    const hasConsoleAccess = isAdmin || (status?.permissions ?? []).some((rule) => rule.effect !== 'Deny')
    return {
      status,
      loading,
      authenticated: Boolean(status?.authenticated),
      isAdmin,
      hasConsoleAccess,
      refresh,
      signOut,
      can,
    }
  }, [status, loading, refresh, signOut])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth 必须在 AuthProvider 内使用')
  return ctx
}
