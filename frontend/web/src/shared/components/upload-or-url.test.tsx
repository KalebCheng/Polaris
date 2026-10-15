import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { UploadOrURL } from './upload-or-url'

/** 包裹 i18n Provider 渲染字段（组件内部使用 useTranslation） */
function renderField(ui: React.ReactElement) {
  return render(<I18nextProvider i18n={i18n}>{ui}</I18nextProvider>)
}

/** 受控宿主：验证输入被正确回传并驱动显示（等价真实表单用法） */
function Harness() {
  const [value, setValue] = useState('')
  return <UploadOrURL value={value} onChange={setValue} />
}

describe('UploadOrURL', () => {
  it('URL 模式：输入即回传最新值', async () => {
    renderField(<Harness />)
    const input = screen.getByRole('textbox') as HTMLInputElement
    await userEvent.type(input, 'https://cdn.example.com/logo.png')
    expect(input.value).toBe('https://cdn.example.com/logo.png')
  })

  it('图片地址渲染预览', () => {
    const { container } = renderField(
      <UploadOrURL value="https://cdn.example.com/logo.png" onChange={() => {}} />,
    )
    const img = container.querySelector('img')
    expect(img?.getAttribute('src')).toBe('https://cdn.example.com/logo.png')
  })

  it('已填值时提供清除入口并回传空串', async () => {
    const onChange = vi.fn()
    renderField(<UploadOrURL value="https://cdn.example.com/a.png" onChange={onChange} />)
    await userEvent.click(screen.getByRole('button', { name: i18n.t('upload.clear') }))
    expect(onChange).toHaveBeenCalledWith('')
  })
})
