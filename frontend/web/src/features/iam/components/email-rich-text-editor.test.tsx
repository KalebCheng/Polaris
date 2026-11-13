import { createRef } from 'react'
import { act, render, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { EmailRichTextEditor, type EmailRichTextEditorHandle } from './email-rich-text-editor'

describe('EmailRichTextEditor', () => {
  it('允许把魔法链接占位符作为 href 插入正文', async () => {
    const ref = createRef<EmailRichTextEditorHandle>()
    const onChange = vi.fn()
    render(
      <I18nextProvider i18n={i18n}>
        <EmailRichTextEditor ref={ref} value="" onChange={onChange} />
      </I18nextProvider>,
    )
    await waitFor(() => expect(ref.current).not.toBeNull())

    act(() => {
      ref.current?.insertHTML('<a href="{{link}}" target="_blank" rel="noopener noreferrer">点击登录</a>')
    })

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(onChange.mock.calls.at(-1)?.[0]).toContain('href="{{link}}"')
  })
})
