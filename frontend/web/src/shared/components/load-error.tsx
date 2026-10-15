import { useTranslation } from 'react-i18next'
import { AlertTriangle } from 'lucide-react'
import { Button } from '@/shared/ui'

interface LoadErrorProps {
  onRetry: () => void
  /** 错误摘要（后端 message 或状态码；不展示堆栈） */
  detail?: string
  compact?: boolean
}

/**
 * 加载失败占位。
 *
 * 存在的理由：**读不到 ≠ 没有**。用户详情的「安全与认证 / 授权 / 关联 / 活动」都是安全语义的视图，
 * 一旦把请求失败静默渲染成空列表，管理员会得出"这个人没有被限制 / 没有权限"的错误结论
 * （真实事故：控制台连的后端进程还是旧版本，`/users/:id/security` 404，页面却显示"当前没有被登录限制的登录来源"）。
 * 因此这些区块在失败时必须显式说"没读到 + 可重试"，绝不退化成空态。
 */
export function LoadError({ onRetry, detail, compact = false }: LoadErrorProps) {
  const { t } = useTranslation()
  return (
    <div
      className={
        compact
          ? 'border-destructive/30 bg-destructive/5 flex items-center justify-between gap-3 rounded-xl border px-3 py-2'
          : 'border-destructive/30 bg-destructive/5 flex flex-col items-start gap-3 rounded-2xl border p-4'
      }
    >
      <p className="text-destructive flex items-center gap-2 text-sm">
        <AlertTriangle className="size-4 shrink-0" />
        <span>{t('common.loadFailed')}</span>
        {detail ? <span className="text-muted-foreground text-xs">{detail}</span> : null}
      </p>
      <Button variant="outline" size="sm" onClick={onRetry}>
        {t('common.retry')}
      </Button>
    </div>
  )
}
