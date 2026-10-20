import { afterEach, describe, expect, it, vi } from 'vitest'
import { setActiveInstitution } from './active-institution'
import { applicationApi, roleApi, userApi } from './api'

/**
 * 回归：组织内资源的写操作必须带上当前工作区组织。
 *
 * 后端 `instOf(c)` 优先取 `?institution=`，否则回退到**登录会话所在组织**。列表接口一直带着它，
 * 但 reset-password / delete 等曾漏带——平台管理员在「非会话组织」的工作区操作时，后端按会话组织
 * 找不到资源（报「用户不存在」）。这里锁死：组织内写操作默认注入工作区组织，显式值优先。
 */
function okFetch() {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: () => Promise.resolve(JSON.stringify({ code: 'OK', message: 'ok', data: {} })),
  } as Response)
}

describe('组织内请求自动携带 ?institution=', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    setActiveInstitution('')
  })

  it('重置密码带上当前工作区组织（否则后端按会话组织查不到用户）', async () => {
    setActiveInstitution('acme')
    const fetchMock = okFetch()
    vi.stubGlobal('fetch', fetchMock)

    await userApi.resetPassword(7, 'newpass')

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/users/7/reset-password')
    expect(url).toContain('institution=acme')
    expect(init.method).toBe('POST')
  })

  it('删除 / 更新同样带上组织', async () => {
    setActiveInstitution('acme')
    const fetchMock = okFetch()
    vi.stubGlobal('fetch', fetchMock)

    await applicationApi.remove(5)
    expect(fetchMock.mock.calls[0][0] as string).toContain('/applications/5')
    expect(fetchMock.mock.calls[0][0] as string).toContain('institution=acme')
  })

  it('显式传入的 institution 优先于当前工作区组织', async () => {
    setActiveInstitution('acme')
    const fetchMock = okFetch()
    vi.stubGlobal('fetch', fetchMock)

    await userApi.get(9, 'other')

    expect(fetchMock.mock.calls[0][0] as string).toContain('/users/9')
    expect(fetchMock.mock.calls[0][0] as string).toContain('institution=other')
  })

  it('平台级资源按资源自身组织定位（角色删除）', async () => {
    setActiveInstitution('acme')
    const fetchMock = okFetch()
    vi.stubGlobal('fetch', fetchMock)

    await roleApi.remove(3, 'built-in')

    expect(fetchMock.mock.calls[0][0] as string).toContain('/roles/3')
    expect(fetchMock.mock.calls[0][0] as string).toContain('institution=built-in')
  })
})
