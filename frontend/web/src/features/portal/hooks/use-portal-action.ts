import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/shared/ui/toast'
import { describeError } from '@/shared/lib/errors'

export function usePortalAction() {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)

  const run = useCallback(
    async (action: () => Promise<unknown>, successKey?: string) => {
      setPending(true)
      try {
        await action()
        if (successKey) toast.success(t(successKey))
        return true
      } catch (error) {
        toast.error(describeError(error, t))
        return false
      } finally {
        setPending(false)
      }
    },
    [t],
  )

  return { pending, run }
}
