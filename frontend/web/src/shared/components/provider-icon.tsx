import {
  Cloud,
  Database,
  Fingerprint,
  Globe,
  HardDrive,
  Mail,
  MessageSquareText,
  Plug,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react'
import { ConnectionIcon, type ConnectionIconSize } from './connection-icon'

/**
 * 提供商图标：与连接模块同一视觉语言。
 * 官方素材（钉钉 / 飞书 / Lark / OIDC）复用连接模块的素材表，其余类型用矢量兜底——
 * 绝不出现空白占位。图标 key 由后端目录（internal/service/provider/catalog.go）声明。
 */
const GLYPHS: Record<string, LucideIcon> = {
  // 类别图标（后端目录 CategoryInfo.Icon 声明）
  signin: Fingerprint,
  mfa: ShieldCheck,
  totp: ShieldCheck,
  sms: MessageSquareText,
  storage: HardDrive,
  // 类别 key 兜底：目录尚未下发 icon 时（例如前端已更新、后端未重启）仍能画出合适的图标，
  // 而不是所有菜单都退化成同一个占位图；与类型图标同名的 key（email / captcha）复用同一图形
  oauth: Fingerprint,
  // 类型图标
  email: Mail,
  aliyun: MessageSquareText,
  captcha: ShieldCheck,
  hcaptcha: ShieldCheck,
  recaptcha: ShieldCheck,
  turnstile: ShieldCheck,
  disk: HardDrive,
  s3: Cloud,
  minio: Database,
  http: Globe,
}

/** 有官方素材的 slug（走连接模块的素材表） */
const OFFICIAL = new Set(['dingtalk', 'wechat', 'feishu', 'lark', 'oidc'])

export function ProviderIcon({ iconKey, size = 24 }: { iconKey: string; size?: ConnectionIconSize }) {
  if (OFFICIAL.has(iconKey)) {
    return <ConnectionIcon icon={{ key: iconKey }} size={size} />
  }
  const Glyph = GLYPHS[iconKey] ?? Plug
  return (
    <Glyph
      aria-hidden="true"
      style={{ width: size, height: size }}
      className="text-muted-foreground shrink-0"
    />
  )
}

/** 中性基底容器：与连接的卡片目录一致（描边 + 圆角 + 居中图标） */
export function ProviderTile({ iconKey, tileSize = 40 }: { iconKey: string; tileSize?: number }) {
  const size = Math.round(tileSize * 0.8) as ConnectionIconSize
  return (
    <span
      className="border-border bg-card inline-flex shrink-0 items-center justify-center rounded-xl border"
      style={{ width: tileSize, height: tileSize }}
    >
      <ProviderIcon iconKey={iconKey} size={size} />
    </span>
  )
}
