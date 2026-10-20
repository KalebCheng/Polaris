import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/shared/ui/toast'
import { describeError } from '@/shared/lib/errors'

/**
 * 变更操作通用逻辑：统一 loading 状态与成功/失败提示。
 * action 抛错时返回 false，调用方据此决定是否关闭对话框。
 */
export function useMutation() {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)

  const run = useCallback(
    async (action: () => Promise<unknown>, successKey = 'common.saved'): Promise<boolean> => {
      setPending(true)
      try {
        await action()
        toast.success(t(successKey))
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
