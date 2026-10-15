import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import i18n from '@/shared/i18n'
import { SchemaFields, type SchemaField } from './schema-fields'

function renderFields(fields: SchemaField[], typeKey?: string) {
  return render(
    <I18nextProvider i18n={i18n}>
      <SchemaFields
        fields={fields}
        values={{}}
        onChange={() => {}}
        i18nPrefix="providers"
        typeKey={typeKey}
      />
    </I18nextProvider>,
  )
}

const endpointField: SchemaField = { key: 'endpoint', label: '端点', group: 'connection', type: 'text' }

/**
 * 回归：标签必须按"类型专属 → 通用 → 目录声明"回退。
 * 曾把三种含义硬拼成一条通用词条（「端点 / 根目录 / 网关地址」），
 * 用户在阿里云短信的向导里看到"根目录"，根本不知道填什么。
 */
describe('SchemaFields 标签解析', () => {
  it('类型专属词条优先于通用词条', () => {
    const { unmount } = renderFields([endpointField], 'aliyun')
    expect(screen.getByText('API endpoint')).toBeInTheDocument()
    unmount()

    renderFields([endpointField], 'local')
    expect(screen.getByText('Storage root path')).toBeInTheDocument()
  })

  it('无类型专属词条时回退到通用词条（不再出现多义拼盘）', () => {
    renderFields([endpointField], 's3')
    expect(screen.getByText('Endpoint')).toBeInTheDocument()
    expect(screen.queryByText(/Root path|Gateway/)).toBeNull()
  })

  it('类型专属提示优先于通用提示', () => {
    const field: SchemaField = {
      key: 'endpoint',
      label: '网关地址',
      group: 'connection',
      type: 'text',
      hint: '平台以 JSON POST 调用',
    }
    renderFields([field], 'http')
    expect(
      screen.getByText('Called with a JSON POST body (phone / code / signName / template)'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Leave blank to use the vendor default endpoint')).toBeNull()
  })

  it('无任何词条时回退目录声明（中文兜底，不会显示原始 key）', () => {
    const field: SchemaField = { key: 'unknown_field', label: '目录里的标签', group: 'basic', type: 'text' }
    renderFields([field])
    expect(screen.getByText('目录里的标签')).toBeInTheDocument()
    expect(screen.queryByText(/providers\.field/)).toBeNull()
  })
})
