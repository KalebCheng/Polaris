import { Globe, Plug } from 'lucide-react'
import { cn } from '@/shared/lib/utils'

/** 图标规格：与后端 catalog 的 `icon` 声明一致（key + 子类型映射） */
export interface ConnectionIconSpec {
  key: string
  by_sub_type?: Record<string, string>
}

/** 图标尺寸白名单：目录/实例用 32–40，行内最小 24，16 仅用于形状简单的图标 */
export type ConnectionIconSize = 16 | 20 | 24 | 32 | 40

/**
 * 官方素材表：slug → 文件名。
 * 素材位于 `public/icons/connections/`，来源与处理方式见该目录 `SOURCES.md`。
 * 未登记的 slug 一律走内联矢量兜底（绝不出现空白占位）。
 */
const ASSETS: Record<string, string> = {
  dingtalk: 'dingtalk.png',
  wechat: 'wechat.png',
  wecom: 'wecom.png',
  feishu: 'feishu.png',
  lark: 'feishu.png', // 用户确认「Lark 与飞书同款」，复用同一素材
  ad: 'ad.png',
  ldap: 'openldap.png',
  mysql: 'mysql.svg',
  postgres: 'postgres.png',
  oidc: 'oidc.png',
}

/** 解析图标 slug：子类型优先于类型默认图标 */
export function resolveIconSlug(
  icon: ConnectionIconSpec | undefined,
  type?: string,
  subType?: string,
): string {
  if (subType && icon?.by_sub_type?.[subType]) {
    return icon.by_sub_type[subType]
  }
  return icon?.key ?? type ?? ''
}

interface ConnectionIconProps {
  icon?: ConnectionIconSpec
  /** 类型 key（catalog 未提供 icon 时的回退） */
  type?: string
  /** 子类型（如 database 的 mysql / postgres） */
  subType?: string
  size?: ConnectionIconSize
  className?: string
}

/**
 * 连接类型图标：官方素材优先，其余内联矢量兜底。
 * 纯装饰（`aria-hidden`）——语义由相邻的文字承担。
 */
export function ConnectionIcon({ icon, type, subType, size = 24, className }: ConnectionIconProps) {
  const slug = resolveIconSlug(icon, type, subType)
  const file = ASSETS[slug]
  if (file) {
    return (
      <img
        src={`/icons/connections/${file}`}
        width={size}
        height={size}
        alt=""
        aria-hidden="true"
        loading="lazy"
        decoding="async"
        className={cn('shrink-0 object-contain', className)}
      />
    )
  }
  const Glyph = slug === 'http' ? Globe : Plug
  return (
    <Glyph
      aria-hidden="true"
      style={{ width: size, height: size }}
      className={cn('text-muted-foreground shrink-0', className)}
    />
  )
}

interface ConnectionTileProps extends ConnectionIconProps {
  /** 基底边长（默认 40px；内部图形按 80% 居中） */
  tileSize?: number
}

/** 中性基底容器：白底 / 暗色底 + 描边 + 圆角，官方素材与兜底矢量视觉高度一致 */
export function ConnectionTile({ tileSize = 40, ...iconProps }: ConnectionTileProps) {
  const size = (iconProps.size ?? Math.round(tileSize * 0.8)) as ConnectionIconSize
  return (
    <span
      className="border-border bg-card inline-flex shrink-0 items-center justify-center rounded-xl border"
      style={{ width: tileSize, height: tileSize }}
    >
      <ConnectionIcon {...iconProps} size={size} />
    </span>
  )
}
