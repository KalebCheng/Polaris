import { BrandMark } from './brand-mark'
import { cn } from '@/shared/lib/utils'

/** 平台名（品牌标识里的字标默认用它，不随界面语言变化） */
export const PLATFORM_NAME = 'Polaris'

interface BrandLockupProps {
  /** 组织自定义 Logo；提供时用「组织 Logo + 名称」，否则用平台字标 */
  src?: string
  /** 展示名称（组织品牌登录页传组织名）；缺省用平台名 */
  name?: string
  /** 字标字号档位：sm＝登录页页眉，md＝顶栏，lg＝营销/空态页 */
  size?: 'sm' | 'md' | 'lg'
  /** 让标识轻微呼吸（认证页用；控制台里保持静止，避免干扰读数） */
  animated?: boolean
  className?: string
}

const textSize: Record<NonNullable<BrandLockupProps['size']>, string> = {
  sm: 'text-[16px]',
  md: 'text-[20px]',
  lg: 'text-[26px]',
}

/**
 * 字标外观：粗字重 + 收敛字距 + **蓝→紫渐变**（与标识光球同族）。
 * 渐变只落在字母上（`bg-clip-text`），光球是独立 SVG，不受影响。
 * 渐变两端的颜色是令牌（`globals.css` 明暗双值 + 认证作用域单独提亮），组件里不写死色值。
 */
const wordmarkSkin =
  'bg-linear-to-r from-[var(--brand-wordmark-from)] to-[var(--brand-wordmark-to)] bg-clip-text font-bold tracking-tight text-transparent'

/**
 * 品牌锁定组合（标识 + 名称）——顶栏 / 门户 / 登录页页眉统一用它，避免各处各拼一遍。
 *
 * 两种形态：
 * 1. **组织品牌**（给了 `src`）：组织 Logo 图 + 组织名称（跟随品牌作用域规则，
 *    控制台永远拿不到组织 Logo，只有门户 / 登录页会传）；
 * 2. **平台字标**：名称就是平台名时，用「字标里光球替换字母 o」的写法（参照 Global 的造型）；
 *    名称是别的（极少见）时退化为「小标识 + 名称」。
 */
export function BrandLockup({ src, name, size = 'md', animated = false, className }: BrandLockupProps) {
  const label = name?.trim() || PLATFORM_NAME
  const markMotion = animated ? 'brand-mark' : undefined

  if (src) {
    return (
      <span className={cn('inline-flex items-center gap-2.5', className)}>
        <img src={src} alt="" className={cn('size-8 shrink-0 object-contain', markMotion)} loading="lazy" />
        <span className={cn('font-bold tracking-tight', textSize[size])}>{label}</span>
      </span>
    )
  }

  if (label === PLATFORM_NAME) {
    return (
      <span className={cn('inline-flex items-center', wordmarkSkin, textSize[size], className)}>
        {/* 光球替换字母 o：与参考视觉同构，但用的是平台自己的冷边暖芯配色 */}
        <span aria-hidden="true">P</span>
        <BrandOrb className={markMotion} />
        <span aria-hidden="true">laris</span>
        <span className="sr-only">{label}</span>
      </span>
    )
  }

  return (
    <span className={cn('inline-flex items-center gap-2.5', className)}>
      <BrandMark className={cn('size-8', markMotion)} />
      <span className={cn('font-bold tracking-tight', textSize[size])}>{label}</span>
    </span>
  )
}

/** 字标里的光球：尺寸随字号走（1.3em），因此 15 / 17 / 22px 下比例一致 */
function BrandOrb({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      aria-hidden="true"
      xmlns="http://www.w3.org/2000/svg"
      className={cn('mx-[0.07em] size-[1.3em] shrink-0', className)}
    >
      <defs>
        <radialGradient id="polaris-wordmark-orb" cx="13" cy="11.5" r="18" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#FFD8A0" />
          <stop offset="0.3" stopColor="#FB923C" />
          <stop offset="0.6" stopColor="#7C5CFA" />
          <stop offset="0.84" stopColor="#5B8DEF" />
          <stop offset="1" stopColor="#22D3EE" />
        </radialGradient>
        <mask id="polaris-wordmark-star-core">
          <rect width="32" height="32" fill="#fff" />
          <circle cx="16" cy="16" r="2.5" fill="#000" />
        </mask>
      </defs>
      <circle cx="16" cy="16" r="16" fill="url(#polaris-wordmark-orb)" />
      <path
        d="M16 5.5 Q20 12 26.5 16 Q20 20 16 26.5 Q12 20 5.5 16 Q12 12 16 5.5Z"
        fill="#fff"
        mask="url(#polaris-wordmark-star-core)"
      />
    </svg>
  )
}
