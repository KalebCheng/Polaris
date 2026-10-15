import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/shared/ui'

/** 复制按钮：用于 client_id / client_secret / 端点地址等 */
export function CopyButton({ value, label }: { value: string; label?: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      toast.success(t('common.copySuccess'))
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error(t('common.operationFailed'))
    }
  }

  return (
    <Button type="button" variant="ghost" size="icon" onClick={copy} aria-label={label ?? t('common.confirm')}>
      {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
    </Button>
  )
}
