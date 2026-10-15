import { useTranslation } from 'react-i18next'
import { ArrowLeft, ArrowRight, Check } from 'lucide-react'
import { Button } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

export interface WizardStep {
  key: string
  /** i18n key：步骤名 */
  titleKey: string
  /** i18n key：步骤说明（讲清这一步在配置什么） */
  descriptionKey?: string
  /** 选填步骤（可在步骤名旁标注「可选」） */
  optional?: boolean
}

interface FormWizardProps {
  steps: WizardStep[]
  /** 当前步骤下标（受控） */
  current: number
  onStepChange: (index: number) => void
  /** 最后一步的提交（「创建」或「保存」） */
  onFinish: () => void
  finishLabelKey: string
  onCancel?: () => void
  cancelLabelKey?: string
  pending?: boolean
  /** 当前步是否满足继续条件；为 false 时禁用「下一步」 */
  canProceed?: boolean
  /** 当前步骤的表单内容 */
  children: React.ReactNode
}

/**
 * 分步表单：顶部步骤条 + 单步内容 + 吸底操作栏（可选右侧常驻栏）。
 *
 * 设计要点（对齐 Auth0 / Okta 的「创建应用」心智）：
 * - **渐进披露**：一屏只呈现当前这一步，字段成组、首屏只问必要信息；
 * - **步骤条可回跳**：已完成步骤点击即可返回修改，不做"只进不退"的强制线性；
 * - **吸底操作**：无论字段多少，「上一步 / 下一步 / 提交」始终在手边；
 * - **实时侧栏**：品牌等有即时反馈的配置，右侧常驻预览，改哪看哪。
 */
export function FormWizard({
  steps,
  current,
  onStepChange,
  onFinish,
  finishLabelKey,
  onCancel,
  cancelLabelKey = 'common.cancel',
  pending = false,
  canProceed = true,
  children,
}: FormWizardProps) {
  const { t } = useTranslation()
  const isFirst = current <= 0
  const isLast = current >= steps.length - 1
  const step = steps[current]

  return (
    <>
      {/*
        吸顶头部：步骤条 + 操作按钮。
        滚动到表单深处时「上一步 / 下一步 / 提交」始终在手边（不必滚到底），
        步骤条也不会滚出视口。负外边距使其横向铺满内容区（与布局 padding 对齐）。
      */}
      <div className="bg-background/85 supports-[backdrop-filter]:bg-background/70 border-border/70 sticky top-0 z-30 -mx-4 border-b px-4 py-3 backdrop-blur-xl sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
          <nav aria-label={t('wizard.steps')} className="min-w-0 flex-1">
            <ol className="flex flex-wrap items-center gap-x-2 gap-y-2">
              {steps.map((item, index) => {
                const state = index < current ? 'done' : index === current ? 'current' : 'upcoming'
                return (
                  <li key={item.key} className="flex min-w-0 flex-1 items-center gap-2 last:flex-none">
                    <button
                      type="button"
                      aria-current={state === 'current' ? 'step' : undefined}
                      onClick={() => onStepChange(index)}
                      className={cn(
                        'group flex min-w-0 items-center gap-2 rounded-full px-1.5 py-1 text-left transition-colors',
                        index !== current && 'hover:bg-accent/50',
                      )}
                    >
                      <span
                        className={cn(
                          'grid size-6 shrink-0 place-items-center rounded-full border text-[11px] font-semibold transition-colors',
                          state === 'done' && 'border-primary/25 bg-primary/10 text-primary',
                          state === 'current' && 'border-primary bg-primary text-primary-foreground',
                          state === 'upcoming' && 'border-border text-muted-foreground',
                        )}
                      >
                        {state === 'done' ? <Check className="size-3.5" /> : index + 1}
                      </span>
                      <span className="min-w-0">
                        <span
                          className={cn(
                            'block truncate text-[13px]',
                            state === 'current' ? 'text-foreground font-semibold' : 'text-muted-foreground',
                          )}
                        >
                          {t(item.titleKey)}
                        </span>
                        {item.optional ? (
                          <span className="text-muted-foreground block text-[10px] leading-tight">
                            {t('wizard.optional')}
                          </span>
                        ) : null}
                      </span>
                    </button>
                    {index < steps.length - 1 ? (
                      <span
                        className={cn(
                          'h-px min-w-3 flex-1 rounded-full transition-colors',
                          index < current ? 'bg-primary/40' : 'bg-border',
                        )}
                      />
                    ) : null}
                  </li>
                )
              })}
            </ol>
          </nav>

          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {onCancel ? (
              <Button type="button" variant="ghost" onClick={onCancel} disabled={pending}>
                {t(cancelLabelKey)}
              </Button>
            ) : null}
            {!isFirst ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => onStepChange(current - 1)}
                disabled={pending}
              >
                <ArrowLeft className="size-4" /> {t('wizard.back')}
              </Button>
            ) : null}
            {isLast ? (
              <Button type="button" onClick={onFinish} disabled={pending}>
                {t(finishLabelKey)}
              </Button>
            ) : (
              <Button
                type="button"
                onClick={() => onStepChange(current + 1)}
                disabled={pending || !canProceed}
              >
                {t('wizard.next')} <ArrowRight className="size-4" />
              </Button>
            )}
          </div>
        </div>
      </div>

      {/* 当前步骤标题 + 内容 */}
      <div className="flex min-w-0 flex-col gap-4 pt-4">
        {step ? (
          <div className="space-y-1">
            <p className="text-muted-foreground text-xs">
              {t('wizard.stepOf', { current: current + 1, total: steps.length })}
            </p>
            <h2 className="text-[15px] font-semibold tracking-tight">{t(step.titleKey)}</h2>
            {step.descriptionKey ? (
              <p className="text-muted-foreground text-xs leading-relaxed">{t(step.descriptionKey)}</p>
            ) : null}
          </div>
        ) : null}
        {children}
      </div>
    </>
  )
}
