import { describe, expect, it } from 'vitest'
import type { Application } from '@/shared/api/types'
import {
  applicationToForm,
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
