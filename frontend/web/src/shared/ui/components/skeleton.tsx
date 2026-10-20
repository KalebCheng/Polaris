import { cn } from '@/shared/lib/utils'

export function Skeleton({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div data-slot="skeleton" className={cn('bg-muted/80 animate-pulse rounded-lg', className)} {...props} />
  )
}
