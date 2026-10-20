import { API_BASE, ApiError, api } from '@/shared/api/client'
import type {
  AppAccount,
  AuditRecord,
  MeResponse,
  PageData,
  ProviderInfo,
  SessionInfo,
  UserIdentity,
} from '@/shared/api/types'

export interface PortalApplication {
  name: string
  display_name: string
  description?: string
  logo?: string
  homepage_url?: string
  /** 一键 SSO：客户端 ID 与已注册回跳地址（为空表示未配置 SSO 入口） */
  client_id?: string
  redirect_uri?: string
}

/**
 * 构造 OIDC 授权请求地址（同源，指向本部署的 /oauth/authorize）。
 * state/nonce 为一次性随机串（防 CSRF 与重放）；应用未登记 client_id/redirect_uri 时返回 undefined。
 */
export function buildAuthorizeURL(app: PortalApplication): string | undefined {
  if (!app.client_id || !app.redirect_uri) return undefined
  const params = new URLSearchParams({
    client_id: app.client_id,
    redirect_uri: app.redirect_uri,
    response_type: 'code',
    scope: 'openid profile email',
    state: randomState(),
    nonce: randomState(),
  })
  return `${window.location.origin}/oauth/authorize?${params.toString()}`
}

function randomState(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

export interface PortalSession extends SessionInfo {
  is_current: boolean
}

export type PersonalAuditRecord = Pick<
  AuditRecord,
  'id' | 'method' | 'request_uri' | 'action' | 'object' | 'client_ip' | 'status_code' | 'created_at'
>

export const portalApi = {
  me: () => api.get<MeResponse>('/me'),
  applications: () => api.get<{ items: PortalApplication[] }>('/me/applications'),
  appAccounts: () => api.get<{ items: AppAccount[] }>('/me/application-accounts'),
  identities: () => api.get<{ items: UserIdentity[] }>('/me/identities'),
  unbindIdentity: (id: number) => api.del<void>(`/me/identities/${id}`),
  identityProviders: () => api.get<{ providers: ProviderInfo[] | null }>('/auth/providers'),
  authorizeIdentityLink: (provider: string) =>
    api.post<{ authorization_url: string; state: string }>(
      `/me/identities/${encodeURIComponent(provider)}/authorize`,
    ),
  sessions: () => api.get<{ items: PortalSession[] }>('/me/sessions'),
  revokeSession: (id: string) => api.del<void>(`/me/sessions/${encodeURIComponent(id)}`),
  revokeAllSessions: () => api.post<void>('/me/sessions/revoke-all'),
  records: (page = 1, size = 20) => api.get<PageData<PersonalAuditRecord>>('/me/records', { page, size }),
  uploadAvatar: async (file: File) => {
    const form = new FormData()
    form.append('file', file)
    const response = await fetch(`${API_BASE}/me/avatar`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
    const payload = (await response.json()) as {
      code?: string
      message?: string
      data?: { avatar: string }
    }
    if (!response.ok || payload.code !== 'OK') {
      throw new ApiError(payload.code ?? 'INTERNAL_ERROR', payload.message ?? '头像上传失败', response.status)
    }
    return payload.data as { avatar: string }
  },
}

export function safeHomepageURL(value?: string): string | undefined {
  if (!value) return undefined
  try {
    const url = new URL(value, window.location.origin)
    if (url.protocol === 'http:' || url.protocol === 'https:') return url.toString()
  } catch {
    return undefined
  }
  return undefined
}
