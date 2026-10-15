import { describe, expect, it } from 'vitest'
import { buildAuthorizeURL, safeHomepageURL } from './portal-api'

describe('safeHomepageURL', () => {
  it('allows relative and HTTP application entry points', () => {
    expect(safeHomepageURL('/launch')).toBe(`${window.location.origin}/launch`)
    expect(safeHomepageURL('https://app.example.com')).toBe('https://app.example.com/')
  })

  it('rejects executable or malformed URL schemes', () => {
    expect(safeHomepageURL('javascript:alert(1)')).toBeUndefined()
    expect(safeHomepageURL('data:text/html,hello')).toBeUndefined()
    expect(safeHomepageURL('')).toBeUndefined()
  })
})

describe('buildAuthorizeURL', () => {
  it('builds a same-origin OIDC authorize request when SSO is configured', () => {
    const url = buildAuthorizeURL({
      name: 'app1',
      display_name: 'App One',
      client_id: 'cid-123',
      redirect_uri: 'https://app.example.com/callback',
    })
    expect(url).toBeDefined()
    const parsed = new URL(url as string)
    expect(parsed.origin).toBe(window.location.origin)
    expect(parsed.pathname).toBe('/oauth/authorize')
    expect(parsed.searchParams.get('client_id')).toBe('cid-123')
    expect(parsed.searchParams.get('redirect_uri')).toBe('https://app.example.com/callback')
    expect(parsed.searchParams.get('response_type')).toBe('code')
    expect(parsed.searchParams.get('scope')).toContain('openid')
    expect(parsed.searchParams.get('state')).toBeTruthy()
    expect(parsed.searchParams.get('nonce')).toBeTruthy()
  })

  it('returns undefined when client_id or redirect_uri is missing', () => {
    expect(buildAuthorizeURL({ name: 'a', display_name: 'A' })).toBeUndefined()
    expect(buildAuthorizeURL({ name: 'a', display_name: 'A', client_id: 'x' })).toBeUndefined()
    expect(buildAuthorizeURL({ name: 'a', display_name: 'A', redirect_uri: 'https://x/cb' })).toBeUndefined()
  })

  it('generates a fresh random state per call', () => {
    const app = { name: 'a', display_name: 'A', client_id: 'x', redirect_uri: 'https://x/cb' }
    const s1 = new URL(buildAuthorizeURL(app) as string).searchParams.get('state')
    const s2 = new URL(buildAuthorizeURL(app) as string).searchParams.get('state')
    expect(s1).not.toBe(s2)
  })
})
