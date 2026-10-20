import * as DialogPrimitive from '@radix-ui/react-dialog'
import { XIcon } from 'lucide-react'
import { cn } from '@/shared/lib/utils'

export const Dialog = DialogPrimitive.Root
export const DialogTrigger = DialogPrimitive.Trigger
export const DialogClose = DialogPrimitive.Close
export const DialogPortal = DialogPrimitive.Portal

export function DialogOverlay({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        'bg-ink/25 fixed inset-0 z-50 backdrop-blur-[3px]',
        'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
        className,
      )}
      {...props}
    />
  )
}

export function DialogContent({
  className,
  innerClassName,
  children,
  showCloseButton = true,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & {
  showCloseButton?: boolean
  /** 覆盖内层可滚动区域（默认 padding / 间距）——抽屉类用法传 "p-0 gap-0" 保持贴边 */
  innerClassName?: string
}) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        className={cn(
          'surface-strong fixed top-1/2 left-1/2 z-50 flex max-h-[calc(100vh-3rem)] w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-2xl sm:max-w-lg',
          className,
        )}
        {...props}
      >
        {/*
          内容放进**内层滚动区**，关闭按钮是它的兄弟节点。
          绝对定位元素若落在滚动容器内，会随内容一起滚——这正是"右上角 ✕ 在弹窗内容滚动时跟着跑"的根因。
          列宽用 grid-cols-1（= minmax(0,1fr)）锁定为容器宽：否则隐式 auto 列会被超长 URL 撑宽、
          溢出后被 overflow-x-hidden 裁掉，导致同一行的复制 / 打开按钮看不见；锁定后超长串正常省略。
        */}
        <div
          className={cn(
            'grid min-h-0 grid-cols-1 gap-4 overflow-x-hidden overflow-y-auto p-6',
            innerClassName,
          )}
        >
          {children}
        </div>
        {showCloseButton ? (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            className="text-muted-foreground hover:bg-accent hover:text-foreground absolute top-4 right-4 rounded-full p-1.5 transition-colors focus:outline-none"
          >
            <XIcon className="size-4" />
            <span className="sr-only">关闭</span>
          </DialogPrimitive.Close>
        ) : null}
      </DialogPrimitive.Content>
    </DialogPortal>
  )
}

export function DialogHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="dialog-header" className={cn('flex flex-col gap-1.5 pr-8', className)} {...props} />
}

export function DialogFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn('flex flex-col-reverse gap-2 sm:flex-row sm:justify-end', className)}
      {...props}
    />
  )
}

export function DialogTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn('text-base font-semibold', className)}
      {...props}
    />
  )
}

export function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  )
}
