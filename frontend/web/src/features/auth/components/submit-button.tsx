import { Button, Spinner, type ButtonProps } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

interface SubmitButtonProps extends ButtonProps {
  /** 提交进行中：按钮缓动收成一个圆，圆内转圈直到结果返回 */
  pending?: boolean
}

/**
 * 登录流程的提交按钮。
 *
 * 提交后按钮从整宽收成一个圆形、文字淡出、圆内转圈——比"文字旁多一个 spinner"更克制，
 * 也不会在收缩时因为文字宽度变化而抖动：文字与 spinner 各占一层绝对定位，只切换透明度，
 * 收缩过程不触发重排。宽度与缓动走 `--motion-pill-*` / `--motion-spring` 令牌。
 */
export function SubmitButton({
  pending = false,
  className,
  children,
  disabled,
  ...props
}: SubmitButtonProps) {
  return (
    <Button
      type="submit"
      data-pending={pending}
      aria-busy={pending || undefined}
      disabled={disabled || pending}
      className={cn(
        // flex（而非 Button 默认的 inline-flex）：收缩后用 mx-auto 才能水平居中
        'relative mx-auto flex h-10 w-full overflow-hidden p-0',
        'transition-[width,background-color,color,box-shadow] duration-[var(--motion-pill-size)] ease-[var(--motion-spring)]',
        'motion-reduce:transition-none',
        pending && 'w-10 disabled:opacity-100',
        className,
      )}
      {...props}
    >
      <span
        className={cn(
          'absolute inset-0 flex items-center justify-center gap-2 px-4 whitespace-nowrap transition-opacity duration-200 motion-reduce:transition-none',
          pending && 'opacity-0',
        )}
      >
        {children}
      </span>
      <span
        aria-hidden="true"
        className={cn(
          'absolute inset-0 flex items-center justify-center opacity-0 transition-opacity duration-200 motion-reduce:transition-none',
          pending && 'opacity-100 delay-150',
        )}
      >
        <Spinner />
      </span>
    </Button>
  )
}
