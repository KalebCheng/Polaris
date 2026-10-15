import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, request } from './client'

function mockFetch(status: number, body: unknown, ok = status >= 200 && status < 300) {
  return vi.fn().mockResolvedValue({
    ok,
    status,
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body)),
  } as Response)
}

describe('api client', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('解包统一响应体中的 data', async () => {
    vi.stubGlobal('fetch', mockFetch(200, { code: 'OK', message: 'ok', data: { name: 'built-in' } }))
    const result = await request<{ name: string }>('/institutions/built-in')
    expect(result.name).toBe('built-in')
  })

  it('HTTP 错误时抛出带业务码的 ApiError', async () => {
    vi.stubGlobal('fetch', mockFetch(401, { code: 'UNAUTHORIZED', message: 'unauthorized' }, false))
    await expect(request('/users')).rejects.toMatchObject({ code: 'UNAUTHORIZED', status: 401 })
  })

  it('HTTP 200 但业务码非 OK 时同样抛错', async () => {
    vi.stubGlobal('fetch', mockFetch(200, { code: 'PERMISSION_DENIED', message: 'denied' }))
    await expect(request('/users')).rejects.toBeInstanceOf(ApiError)
  })

  it('无响应体时返回 undefined（例如 204）', async () => {
    vi.stubGlobal('fetch', mockFetch(200, undefined))
    await expect(request('/auth/logout')).resolves.toBeUndefined()
  })

  it('拼接查询串时跳过空值', async () => {
    const fetchMock = mockFetch(200, { code: 'OK', message: 'ok', data: { items: [], total: 0 } })
    vi.stubGlobal('fetch', fetchMock)
    await api.get('/users', { institution: 'built-in', keyword: '', page: 1 })
    const url = fetchMock.mock.calls[0][0] as string
    expect(url).toContain('institution=built-in')
    expect(url).toContain('page=1')
    expect(url).not.toContain('keyword=')
  })

  it('携带 Cookie（credentials: include）以复用会话', async () => {
    const fetchMock = mockFetch(200, { code: 'OK', message: 'ok', data: {} })
    vi.stubGlobal('fetch', fetchMock)
    await request('/me')
    expect((fetchMock.mock.calls[0][1] as RequestInit).credentials).toBe('include')
  })
})
