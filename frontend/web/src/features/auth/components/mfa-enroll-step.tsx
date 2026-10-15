import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, KeyRound, ScanLine } from 'lucide-react'
import { Button, InputOTP, InputOTPGroup, InputOTPSlot, Spinner } from '@/shared/ui'
import { QrCode } from '@/shared/components/qr-code'
import { CopyButton } from '@/shared/components/copy-button'

interface MfaEnrollStepProps {
  /** 待写入验证器 App 的密钥与 otpauth 链接（来自 /auth/mfa/setup） */
  secret: string
  url: string
  pending: boolean
  onSubmit: (code: string) => void
  onBack: () => void
}

/**
 * 组织强制二次验证时的**绑定步骤**（独立一屏）。
 *
 * 排版原则（对齐 Casdoor `MfaVerifySetupForm` 的"扫码为主"）：
 *   1. 二维码居中，一眼就知道要扫；
 *   2. 紧跟"输入验证器显示的 6 位动态码"——扫码与验证在同一屏，不必来回切页；
 *   3. 密钥与 otpauth 链接收进"无法扫码？"里，需要时才展开，避免一屏三行灰底 code 块。
 */
export function MfaEnrollStep({ secret, url, pending, onSubmit, onBack }: MfaEnrollStepProps) {
  const { t } = useTranslation()
  const [code, setCode] = useState('')
  const [manual, setManual] = useState(false)

  return (
    <form
      className="space-y-5"
      onSubmit={(event) => {
        event.preventDefault()
        if (code.length < 6) return
        onSubmit(code)
      }}
    >
      <div className="space-y-1 text-center">
        <div className="bg-muted mx-auto flex size-10 items-center justify-center rounded-full">
          <ScanLine className="text-muted-foreground size-5" />
        </div>
        <p className="text-sm font-medium">{t('auth.enrollTitle')}</p>
        <p className="text-muted-foreground text-xs">{t('auth.mfaEnrollHint')}</p>
      </div>

      <div className="flex flex-col items-center gap-3">
        <QrCode value={url} />
        <p className="text-muted-foreground text-center text-xs">{t('auth.mfaScanHint')}</p>
      </div>

      <div className="space-y-2">
        {/* 标题已说明要填验证器里的 6 位码，此处不再重复标签 */}
        <InputOTP
          id="mfa-enroll-code"
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
      </div>

      <Button type="submit" className="h-10 w-full" disabled={pending || code.length < 6}>
        {pending ? <Spinner /> : null}
        {t('auth.enrollSubmit')}
      </Button>

      <div className="flex items-center justify-between text-xs">
        <Button type="button" variant="link" size="sm" className="px-0" onClick={onBack}>
          <ArrowLeft className="size-3.5" /> {t('auth.mfaBack')}
        </Button>
        <Button
          type="button"
          variant="link"
          size="sm"
          className="px-0"
          onClick={() => setManual((value) => !value)}
        >
          <KeyRound className="size-3.5" /> {t('auth.enrollManual')}
        </Button>
      </div>

      {manual ? (
        <div className="bg-muted/40 space-y-2 rounded-lg border p-3">
          <p className="text-[11px] font-medium">{t('auth.mfaManual')}</p>
          <div className="bg-background flex items-center gap-2 rounded-md border px-2 py-1.5">
            <code className="min-w-0 flex-1 truncate font-mono text-xs">{secret}</code>
            <CopyButton value={secret} label={t('auth.mfaSecret')} />
          </div>
          <div className="bg-background flex items-center gap-2 rounded-md border px-2 py-1.5">
            <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{url}</code>
            <CopyButton value={url} label={t('auth.mfaUrl')} />
          </div>
          <p className="text-muted-foreground text-[11px]">{t('auth.mfaManualHint')}</p>
        </div>
      ) : null}
    </form>
  )
}
