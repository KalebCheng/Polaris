import * as React from 'react'
import { OTPInput, OTPInputContext } from 'input-otp'
import { cn } from '@/shared/lib/utils'

/**
 * 一格一格的验证码输入（shadcn `input-otp` 形态）。
 *
 * 相比单个输入框：位数一眼可见、自动跳到下一格、支持整串粘贴、
 * 填满即触发 `onComplete`（用于"输完即提交"）。
 */
export function InputOTP({
  className,
  containerClassName,
  ...props
}: React.ComponentProps<typeof OTPInput> & { containerClassName?: string }) {
  return (
    <OTPInput
      data-slot="input-otp"
      containerClassName={cn('flex items-center gap-2 has-disabled:opacity-50', containerClassName)}
      className={cn('disabled:cursor-not-allowed', className)}
      {...props}
    />
  )
}

export function InputOTPGroup({ className, ...props }: React.ComponentProps<'div'>) {
  // 格与格之间留间隙：每格是**独立**的圆角输入块，而不是共享边框连成一条
  return <div data-slot="input-otp-group" className={cn('flex items-center gap-2', className)} {...props} />
}

export function InputOTPSlot({
  index,
  className,
  ...props
}: React.ComponentProps<'div'> & { index: number }) {
  const inputOTPContext = React.useContext(OTPInputContext)
  const slot = inputOTPContext?.slots[index]
  const char = slot?.char ?? ''
  const isActive = slot?.isActive ?? false
  const hasFakeCaret = slot?.hasFakeCaret ?? false
  return (
    <div
      data-slot="input-otp-slot"
      data-active={isActive}
      className={cn(
        'border-input relative flex size-11 items-center justify-center rounded-md border text-lg font-medium shadow-xs transition-all outline-none',
        'data-[active=true]:border-ring data-[active=true]:ring-2 data-[active=true]:ring-ring/40',
        className,
      )}
      {...props}
    >
      {char}
      {hasFakeCaret ? (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
          <div className="bg-foreground h-5 w-px animate-pulse duration-1000" />
        </div>
      ) : null}
    </div>
  )
}

/** 组间分隔（如 3-3 分组的验证码） */
export function InputOTPSeparator({ ...props }: React.ComponentProps<'div'>) {
  return (
    <div data-slot="input-otp-separator" role="separator" className="text-muted-foreground" {...props}>
      -
    </div>
  )
}
