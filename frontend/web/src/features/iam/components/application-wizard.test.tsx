import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { ApplicationWizard } from './application-wizard'
import { emptyApplicationForm } from './application-form'

/** 渲染向导（步骤内容只依赖 props，无需真实接口） */
function renderWizard() {
  return render(
    <I18nextProvider i18n={i18n}>
      <ApplicationWizard
        form={{ ...emptyApplicationForm, code: 'portal', name: 'Portal' }}
        onChange={vi.fn()}
        editing={null}
        providers={[]}
        certs={[]}
        groups={[]}
        onFinish={vi.fn()}
        finishLabelKey="common.save"
      />
    </I18nextProvider>,
  )
}

/** 步骤条上的按钮（顺序与向导步骤一一对应） */
function stepperButtons() {
  const nav = screen.getByRole('navigation', { name: i18n.t('wizard.steps') })
  return within(nav).getAllByRole('button')
}

describe('ApplicationWizard', () => {
  it('「品牌主题」只出现在「基本信息」这一步，切换步骤不重复出现', async () => {
    renderWizard()
    const themeTitle = i18n.t('applications.sectionTheme')
    expect(screen.getByText(themeTitle)).toBeInTheDocument()

    const buttons = stepperButtons()
    expect(buttons).toHaveLength(5)

    const otherSteps = [
      [1, 'applications.stepSigninHint'],
      [2, 'applications.stepSignupHint'],
      [3, 'applications.stepClientHint'],
      [4, 'applications.stepSecurityHint'],
    ] as const

    const limitLabel = i18n.t('applications.signinErrorLimit')
    const waitLabel = i18n.t('applications.signinRetryWaitMinutes')
    expect(screen.queryByText(limitLabel)).toBeNull()

    for (const [index, hintKey] of otherSteps) {
      await userEvent.click(buttons[index])
      // 已切到该步（步骤说明出现），品牌主题不再出现
      expect(screen.getByText(i18n.t(hintKey))).toBeInTheDocument()
      expect(screen.queryByText(themeTitle)).toBeNull()
    }

    // 末步「安全与令牌」承载登录防护（失败次数限制 / 重试等待时间）
    expect(screen.getByText(limitLabel)).toBeInTheDocument()
    expect(screen.getByText(waitLabel)).toBeInTheDocument()

    // 回到第一步仍可编辑品牌主题
    await userEvent.click(buttons[0])
    expect(screen.getByText(themeTitle)).toBeInTheDocument()
  })
})
