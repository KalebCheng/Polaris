import { Loader2 } from 'lucide-react'
import { cn } from '@/shared/lib/utils'

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn('size-4 animate-spin', className)} aria-hidden />
}
