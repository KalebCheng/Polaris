import * as React from 'react'
import { cn } from '@/shared/lib/utils'

export function Textarea({ className, ...props }: React.ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        'border-input/70 bg-card flex min-h-20 w-full rounded-xl border px-3.5 py-2.5 text-[13px] transition-[color,box-shadow,border-color] outline-none',
        'placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50',
        'focus-visible:border-ring focus-visible:ring-ring/25 focus-visible:ring-4',
        'aria-invalid:border-destructive aria-invalid:ring-destructive/20',
        'field-sizing-content',
        className,
      )}
      {...props}
    />
  )
}
