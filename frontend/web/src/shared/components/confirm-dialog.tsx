import { useTranslation } from 'react-i18next'
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/shared/ui'

interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  titleKey: string
  descriptionKey: string
  descriptionParams?: Record<string, unknown>
  confirmLabelKey?: string
  destructive?: boolean
  loading?: boolean
  onConfirm: () => void
}

/** 通用二次确认对话框：避免各页重复拼装危险操作确认 */
export function ConfirmDialog({
  open,
  onOpenChange,
  titleKey,
  descriptionKey,
  descriptionParams,
  confirmLabelKey = 'common.confirm',
  destructive = true,
  loading = false,
  onConfirm,
}: ConfirmDialogProps) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(titleKey)}</DialogTitle>
          <DialogDescription>{t(descriptionKey, descriptionParams)}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button variant={destructive ? 'destructive' : 'default'} disabled={loading} onClick={onConfirm}>
            {t(confirmLabelKey)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
