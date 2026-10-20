/**
 * 第三方 SSO 发起前的回跳目标。
 *
 * OIDC 应用登录页的 client_id / redirect_uri / state 等参数只存在于当前登录 URL；
 * 一旦跳到外部 IdP 再回来，登录页本身已经不在。必须把完整的 /oauth/authorize
 * 地址交给后端随 SSO state 暂存，否则回调后只会按默认角色落地到门户。
 */
export function ssoReturnTarget(oauthQuery: URLSearchParams, from: string): string {
  if (!oauthQuery.get('client_id') || !oauthQuery.get('redirect_uri')) return from
  return `/oauth/authorize?${oauthQuery.toString()}`
}
