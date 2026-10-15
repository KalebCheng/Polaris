import type { Application } from '@/shared/api/types'

/**
 * 构造「可直接打开」的授权链接：带 client_id（以及该应用已登记的首个 redirect_uri）。
 *
 * 背景：裸 /oauth/authorize 不带 client_id 时，登录页会按「组织默认应用」解析
 * （见后端 resolveApplicationOptional）——所以直接访问授权端点看到的是默认应用
 * （内置应用），而不是刚创建的那个。带上 client_id 才会进入本应用的登录页。
 *
 * host 取 console 同源地址：生产由 nginx 同源托管 SPA 与 /oauth，此时 origin 与 issuer 一致；
 * dev 下 Vite 亦代理 /oauth，故这里用同源地址对两种环境都成立。
 */
export function buildAuthorizeUrl(app: Pick<Application, 'client_id' | 'redirect_uris' | 'scopes'>): string {
  const params = new URLSearchParams()
  params.set('client_id', app.client_id)
  params.set('response_type', 'code')
  // openid 必带；其余取应用已配置 scope，去重后按空格拼接（OIDC 约定）
  const scope = new Set(['openid', ...(app.scopes ?? []).map((item) => item.name).filter(Boolean)])
  params.set('scope', [...scope].join(' '))
  const redirect = app.redirect_uris?.find((uri) => uri.trim() !== '')
  if (redirect) params.set('redirect_uri', redirect)
  return `${window.location.origin}/oauth/authorize?${params.toString()}`
}
