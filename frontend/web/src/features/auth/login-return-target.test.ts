import { describe, expect, it } from 'vitest'
import { ssoReturnTarget } from './login-return-target'

describe('ssoReturnTarget', () => {
  it('OIDC 应用登录经第三方 SSO 后仍续走原始授权请求', () => {
    const oauthQuery = new URLSearchParams({
      client_id: 'netbird-dashboard',
      redirect_uri: 'https://netbird.example.com/oauth2/callback',
      response_type: 'code',
      state: 'upstream-state',
    })

    expect(ssoReturnTarget(oauthQuery, '')).toBe(`/oauth/authorize?${oauthQuery.toString()}`)
  })

  it('非 OIDC 登录保留显式 from 回跳', () => {
    expect(ssoReturnTarget(new URLSearchParams(), '/portal/profile')).toBe('/portal/profile')
  })
})
