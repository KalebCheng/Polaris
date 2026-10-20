import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, ShieldCheck } from 'lucide-react'
import { Button, Checkbox, InputOTP, InputOTPGroup, InputOTPSlot, Input, Label } from '@/shared/ui'
import { SubmitButton } from './submit-button'

interface MfaVerifyStepProps {
  /** 组织配置的「记住此设备」时长（小时）；0 = 不支持该能力，不展示复选框 */
  rememberHours: number
  pending: boolean
  /** 提交动态码 / 恢复码；remember 仅在动态码模式下有意义 */
  onSubmit: (code: string, remember: boolean) => void
  /** 返回上一步（重新输入账号密码） */
  onBack: () => void
  /** 是否接受恢复码（短信 MFA 没有恢复码） */
  allowRecovery?: boolean
  /** 额外字段（如人机校验挑战）由调用方注入，保持本组件只关心第二因子 */
  children?: React.ReactNode
}

/**
 * 登录流程的第二因子步骤（**独立一屏**）。
 *
 * 对齐成熟产品（Casdoor `MfaVerify`）的做法：账号密码提交后**切屏**要求第二因子，
 * 而不是在同一个表单里再塞一个「动态码」字段、让用户把账号密码再提交一遍。
 *
 * 交互要点：
 *   - 验证码输入框自动聚焦、只收数字，**填满 6 位自动提交**（这是用户最省事的路径）；
 *   - Enter 可提交；「记住此设备 N 小时」仅在组织配置了时长时出现；
 *   - 验证器丢了可切「使用恢复码」（后端 `verifySecondFactor` 同样接受恢复码）；
 *   - 随时可「返回上一步」。
 */
export function MfaVerifyStep({
  rememberHours,
  pending,
  onSubmit,
  onBack,
  allowRecovery = true,
  children,
}: MfaVerifyStepProps) {
  const { t } = useTranslation()
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState(false)
  const [remember, setRemember] = useState(false)
  // 同一个 6 位码只自动提交一次（避免重渲染重复触发）
  const autoSubmitted = useRef('')

  const digitsOnly = !recovery
  const complete = digitsOnly ? code.length === 6 : code.length >= 6

  useEffect(() => {
    if (!digitsOnly || !complete || pending) return
    if (autoSubmitted.current === code) return
    autoSubmitted.current = code
    onSubmit(code, remember)
  }, [code, complete, digitsOnly, pending, onSubmit, remember])

  const switchMode = (next: boolean) => {
    setRecovery(next)
    setCode('')
    autoSubmitted.current = ''
  }

  return (
    <form
      className="space-y-4"
      onSubmit={(event) => {
        event.preventDefault()
        if (!complete) return
        onSubmit(code, remember)
      }}
    >
      <div className="space-y-1 text-center">
        <div className="bg-muted mx-auto flex size-10 items-center justify-center rounded-full">
          <ShieldCheck className="text-muted-foreground size-5" />
        </div>
        <p className="text-sm font-medium">
          {recovery ? t('auth.mfaRecoveryTitle') : t('auth.mfaStepTitle')}
        </p>
        <p className="text-muted-foreground text-xs">
          {recovery ? t('auth.mfaRecoveryHint') : t('auth.mfaVerifyHint')}
        </p>
      </div>

      <div className="space-y-2">
        {/* 6 位动态码：不额外加标签——上方标题与提示语已说明要填什么，
            可访问名称由 InputOTP 的 aria-label 提供（视觉上少一行重复信息）。
            恢复码长度不固定、语义也不同，保留标签。 */}
        {recovery ? <Label htmlFor="mfa-code">{t('auth.mfaRecoveryCode')}</Label> : null}
        {digitsOnly ? (
          /* 一格一格：位数一眼可见、支持整串粘贴、填满即提交 */
          <InputOTP
            id="mfa-code"
            autoFocus
            maxLength={6}
            value={code}
            onChange={(value) => setCode(value.replace(/\D/g, ''))}
            containerClassName="justify-center"
            aria-label={t('auth.mfaCode')}
          >
            <InputOTPGroup>
              <InputOTPSlot index={0} />
              <InputOTPSlot index={1} />
              <InputOTPSlot index={2} />
              <InputOTPSlot index={3} />
              <InputOTPSlot index={4} />
              <InputOTPSlot index={5} />
            </InputOTPGroup>
          </InputOTP>
        ) : (
          <Input
            id="mfa-code"
            autoFocus
            value={code}
            onChange={(event) => setCode(event.target.value)}
            maxLength={20}
            placeholder={t('auth.mfaRecoveryCode')}
            className="h-10 font-mono"
          />
        )}
        {digitsOnly ? (
          <p className="text-muted-foreground text-[11px]">{t('auth.mfaAutoSubmitHint')}</p>
        ) : null}
      </div>

      {rememberHours > 0 && digitsOnly ? (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={remember} onCheckedChange={(value) => setRemember(Boolean(value))} />
          <span className="text-muted-foreground">{t('auth.rememberDevice', { hours: rememberHours })}</span>
        </label>
      ) : null}

      {children}

      <SubmitButton pending={pending} disabled={!complete}>
        {t('auth.mfaVerifySubmit')}
      </SubmitButton>

      <div className="flex items-center justify-between text-xs">
        <Button type="button" variant="link" size="sm" className="px-0" onClick={onBack}>
          <ArrowLeft className="size-3.5" /> {t('auth.mfaBack')}
        </Button>
        {allowRecovery ? (
          <Button
            type="button"
            variant="link"
            size="sm"
            className="px-0"
            onClick={() => switchMode(!recovery)}
          >
            {recovery ? t('auth.mfaUseTotp') : t('auth.mfaUseRecovery')}
          </Button>
        ) : (
          <span />
        )}
      </div>
    </form>
  )
}
