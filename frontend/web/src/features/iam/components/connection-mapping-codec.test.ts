import { describe, expect, it } from 'vitest'
import { decodeMapping, decodeRule, encodeMapping, encodeRule, emptyRule } from './connection-mapping-codec'

describe('字段映射编码', () => {
  it('历史数据（无前缀单值）解析为源字段', () => {
    expect(decodeRule('mail')).toEqual({ mode: 'field', fields: ['mail'], constant: '', unmapped: false })
  })

  it('备选源字段（管道分隔）往返无损', () => {
    const rule = decodeRule('mobile|email')
    expect(rule.fields).toEqual(['mobile', 'email'])
    expect(encodeRule(rule)).toBe('mobile|email')
  })

  it('固定值往返无损', () => {
    const rule = decodeRule('=远程办公')
    expect(rule.mode).toBe('constant')
    expect(rule.constant).toBe('远程办公')
    expect(encodeRule(rule)).toBe('=远程办公')
  })

  it('未映射的行不落库（开关关闭即删除该键）', () => {
    const rule = { ...emptyRule(), fields: ['mail'], unmapped: true }
    expect(encodeRule(rule)).toBe('')
    expect(encodeMapping({ email: rule })).toEqual({})
  })

  it('未填写的行不落库（不产生空配置）', () => {
    expect(encodeRule(emptyRule())).toBe('')
    expect(encodeRule({ mode: 'constant', fields: [''], constant: '   ', unmapped: false })).toBe('')
  })

  it('整表往返：只保留有效行', () => {
    const stored = { name: 'sAMAccountName', email: 'mail|uid', location: '=远程' }
    expect(encodeMapping(decodeMapping(stored))).toEqual(stored)
  })
})
