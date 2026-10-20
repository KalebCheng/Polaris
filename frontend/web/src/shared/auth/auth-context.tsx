import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { authApi } from '@/shared/api/auth'
import type { AuthStatus } from '@/shared/api/types'

interface AuthContextValue {
  status: AuthStatus | null
  loading: boolean
  authenticated: boolean
  isAdmin: boolean
  /** Whether the user may enter the administration console. */
  hasConsoleAccess: boolean
  refresh: () => Promise<void>
  signOut: () => Promise<void>
  /** Check a resource/action permission. */
  can: (resource: string, action: string) => boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [loading, setLoading] = useState(true)

  // Avoid duplicate bootstrap requests under StrictMode and repeated mounts.
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
      // Always clear local auth state, even if the server session is already gone.
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
    // Delegated administrators can enter the console; end users only use the portal.
    const hasConsoleAccess = isAdmin || (status?.permissions ?? []).some((rule) => rule.effect !== 'Deny')
    return {
      status,
      loading,
      authenticated: Boolean(status?.authenticated && !status?.mfa_pending),
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
