import { Avatar, AvatarFallback, AvatarImage } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

interface UserAvatarProps {
  user: { name: string; display_name?: string; avatar?: string }
  className?: string
}

/**
 * 用户头像：有图显示图，无图回退到首字母。
 * 列表与详情共用一份实现，避免两处各写一遍（含 fallback 的取值口径）。
 */
export function UserAvatar({ user, className }: UserAvatarProps) {
  const label = user.display_name || user.name || '?'
  return (
    <Avatar className={cn('size-8', className)}>
      {user.avatar ? <AvatarImage src={user.avatar} alt={label} /> : null}
      <AvatarFallback>{label.slice(0, 1).toUpperCase()}</AvatarFallback>
    </Avatar>
  )
}
