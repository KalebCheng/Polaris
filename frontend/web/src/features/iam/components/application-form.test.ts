import { describe, expect, it } from 'vitest'
import type { Application } from '@/shared/api/types'
import {
  accessToFormValue,
  applicationToForm,
  emptyApplicationAccess,
  emptyApplicationForm,
  formToApplication,
  formToApplicationValidation,
} from './application-form'

/** 测试用应用（只填与本用例相关的字段，其余走 DTO 的默认回显） */
function app(over: Partial<Application>): Application {
  return { id: 1, institution_id: 1, code: 'portal', name: '门户', ...over } as Application
}

/** 可提交的最小表单（含合法标识，避免先被其他校验拦下） */
const baseForm = { ...emptyApplicationForm, code: 'portal', name: 'Portal' }

describe('登录失败限制字段', () => {
  it('新应用默认对齐 Casdoor：5 次 / 15 分钟', () => {
    expect(emptyApplicationForm.signin_error_limit).toBe(5)
    expect(emptyApplicationForm.signin_retry_wait_minutes).toBe(15)
  })

  it('编辑回显保留显式值', () => {
    const form = applicationToForm(app({ signin_error_limit: 3, signin_retry_wait_minutes: 30 }))
    expect(form.signin_error_limit).toBe(3)
    expect(form.signin_retry_wait_minutes).toBe(30)
  })

  it('编辑回显保留 0（0 = 沿用平台默认，不能被默认值吞掉）', () => {
    const form = applicationToForm(app({ signin_error_limit: 0, signin_retry_wait_minutes: 0 }))
    expect(form.signin_error_limit).toBe(0)
    expect(form.signin_retry_wait_minutes).toBe(0)
  })

  it('提交载荷原样带出两项配置', () => {
    const payload = formToApplication({ ...baseForm, signin_error_limit: 8, signin_retry_wait_minutes: 45 })
    expect(payload.signin_error_limit).toBe(8)
    expect(payload.signin_retry_wait_minutes).toBe(45)
  })

  it('负数被校验拦下（后端另有校验与 CHECK 约束）', () => {
    const negative = formToApplicationValidation({ ...baseForm, signin_error_limit: -1 })
    expect(negative).toEqual({ ok: false, reason: 'applications.signinLimitNegative' })
    const negativeWait = formToApplicationValidation({ ...baseForm, signin_retry_wait_minutes: -5 })
    expect(negativeWait).toEqual({ ok: false, reason: 'applications.signinLimitNegative' })
  })
})

describe('访问授权（向导第 6 步的独立表单值）', () => {
  it('默认全员可访问且名单为空', () => {
    expect(emptyApplicationAccess.mode).toBe('all')
    expect(emptyApplicationAccess.users).toEqual([])
    expect(emptyApplicationAccess.groups).toEqual([])
    expect(emptyApplicationAccess.departments).toEqual([])
  })

  it('接口返回的授权全景转换为表单值（三类主体取标识名）', () => {
    const value = accessToFormValue({
      access_mode: 'restricted',
      users: [{ name: 'zhangsan', display_name: '张三', missing: false }],
      groups: [{ name: 'vpn-users', display_name: 'VPN 用户', missing: false }],
      departments: [{ name: 'tech', display_name: '技术部', path: '技术部', missing: false }],
    })
    expect(value.mode).toBe('restricted')
    expect(value.users).toEqual(['zhangsan'])
    expect(value.groups).toEqual(['vpn-users'])
    expect(value.departments).toEqual(['tech'])
  })

  it('未知访问模式按默认（全员可访问）解读，不产生"半个受限"状态', () => {
    const value = accessToFormValue({ access_mode: '', users: [], groups: [], departments: [] })
    expect(value.mode).toBe('all')
  })

  it('访问授权不进入应用主表单提交体（避免与 /access 端点形成两条写入路径）', () => {
    const payload = formToApplication({ ...baseForm, code: 'portal' }) as Record<string, unknown>
    expect(payload.access_mode).toBeUndefined()
    expect(payload.access_users).toBeUndefined()
    expect(payload.access_groups).toBeUndefined()
    expect(payload.access_departments).toBeUndefined()
  })
})
