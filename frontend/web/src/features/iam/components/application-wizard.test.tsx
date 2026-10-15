import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import { ApplicationWizard } from './application-wizard'
import {
  emptyApplicationAccess,
  emptyApplicationForm,
  type ApplicationAccessFormValue,
} from './application-form'

// 第 6 步进入时会拉"可授权主体候选"（按组织）；本用例只关心步骤隔离，候选给空集合即可。
vi.mock('../api', () => ({
  applicationApi: {
    subjectOptions: vi.fn(async () => ({ items: [] })),
  },
}))

/** 渲染向导（步骤内容只依赖 props，无需真实接口） */
function renderWizard(access: ApplicationAccessFormValue = emptyApplicationAccess) {
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
        access={access}
        onAccessChange={vi.fn()}
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
    expect(buttons).toHaveLength(6)

    const otherSteps = [
      [1, 'applications.stepSigninHint'],
      [2, 'applications.stepSignupHint'],
      [3, 'applications.stepClientHint'],
      [4, 'applications.stepSecurityHint'],
      [5, 'applications.stepAccessHint'],
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

    // 末步「授权与访问」只承载访问授权：不出现「安全与令牌」的登录防护字段
    expect(screen.queryByText(limitLabel)).toBeNull()
    expect(screen.queryByText(waitLabel)).toBeNull()

    // 回到「安全与令牌」仍可看到登录防护字段
    await userEvent.click(buttons[4])
    expect(screen.getByText(limitLabel)).toBeInTheDocument()
    expect(screen.getByText(waitLabel)).toBeInTheDocument()
    // 且此时访问授权的受限内容不出现（步骤内容必须写在步骤条件内）
    expect(screen.queryByText(i18n.t('applications.accessTypes'))).toBeNull()

    // 回到第一步仍可编辑品牌主题
    await userEvent.click(buttons[0])
    expect(screen.getByText(themeTitle)).toBeInTheDocument()
  })

  it('访问授权只在第 6 步出现：默认「全员可访问」，关闭后才是三类主体选择器', async () => {
    renderWizard()
    // 未进入第 6 步时，访问授权的任何内容都不渲染
    expect(screen.queryByText(i18n.t('applications.sectionAccess'))).toBeNull()

    await userEvent.click(stepperButtons()[5])
    expect(screen.getByText(i18n.t('applications.sectionAccess'))).toBeInTheDocument()
    expect(screen.getByText(i18n.t('applications.accessAll'))).toBeInTheDocument()
    // 默认全员可访问：不显示授权类型与名单
    expect(screen.queryByText(i18n.t('applications.accessUsers'))).toBeNull()
    expect(screen.queryByText(i18n.t('applications.accessTypes'))).toBeNull()
  })

  it('受限 + 空名单：只给引导空态、不阻止提交（Spec Q4）', async () => {
    renderWizard({ mode: 'restricted', users: [], groups: [], departments: [] })
    await userEvent.click(stepperButtons()[5])

    expect(screen.getByText(i18n.t('applications.accessListEmpty'))).toBeInTheDocument()
    for (const key of [
      'applications.accessUsers',
      'applications.accessGroups',
      'applications.accessDepartments',
    ]) {
      expect(screen.getByText(i18n.t(key))).toBeInTheDocument()
    }
    // 「空名单可保存」：提交按钮可用（拦截会违背用户明确要求）
    expect(screen.getByRole('button', { name: i18n.t('common.save') })).toBeEnabled()
  })
})
