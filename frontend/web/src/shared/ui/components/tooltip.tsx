import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { cn } from '@/shared/lib/utils'

export const TooltipProvider = TooltipPrimitive.Provider
export const Tooltip = TooltipPrimitive.Root
export const TooltipTrigger = TooltipPrimitive.Trigger

export function TooltipContent({
  className,
  sideOffset = 4,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          'bg-ink text-ink-foreground z-50 w-fit rounded-lg px-2.5 py-1.5 text-[11px] text-balance',
          className,
        )}
        {...props}
      />
    </TooltipPrimitive.Portal>
  )
}
