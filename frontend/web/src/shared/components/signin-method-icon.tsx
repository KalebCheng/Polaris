import { KeyRound, Link2, Mail, Plug, Smartphone, type LucideIcon } from 'lucide-react'

/**
 * 登录方式图标：**唯一定义处**（控制台向导、登录页、预览共用同一套）。
 *
 * 语义取向：
 * - 密码 → 钥匙（凭证）；验证码 → 邮件 / 手机；魔法链接 → 链接；第三方 → 插头（提供方接入，与提供方表单一致）。
 */
const SIGNIN_METHOD_ICONS: Record<string, LucideIcon> = {
  Password: KeyRound,
  EmailCode: Mail,
  SMSCode: Smartphone,
  MagicLink: Link2,
  Provider: Plug,
}

/** 登录方式图标；未知方式回退到钥匙，避免出现空白占位 */
export function SigninMethodIcon({ name, className }: { name: string; className?: string }) {
  const Icon = SIGNIN_METHOD_ICONS[name] ?? KeyRound
  return <Icon className={className ?? 'size-4'} aria-hidden="true" />
}
