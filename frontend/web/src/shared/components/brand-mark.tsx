import { cn } from '@/shared/lib/utils'

interface BrandMarkProps {
  className?: string
  /** 无障碍描述；设置为纯装饰时可传 aria-hidden */
  title?: string
  /** 组织自定义 Logo 地址；提供时以其为先，缺省回退内置矢量标识 */
  src?: string
}

/**
 * Polaris 品牌标识（2026-10-15 重构）。
 *
 * 造型语义来自两条线：
 * - **北极星**（Polaris 的本来含义）：四角星，中心留孔代表"被指向的身份核心"；
 * - **参考视觉的渐变光球**：冷色边缘（青→靛）裹住暖色核心（橙→琥珀），像一颗发光的天体。
 *
 * 于是标识＝星在球上：白星压在光球上，星孔正好透出最亮的琥珀核心——
 * 一眼能认出"星"，也保留参考图那种"发光球体"的质感。
 *
 * 纯矢量，16px 下仍清晰可辨（favicon 用的是同一造型的 `public/icon.svg`）。
 * 渐变与遮罩的 id 在同一页面重复出现是安全的：同名的所有实例定义完全相同。
 */
export function BrandMark({ className, title = 'Polaris', src }: BrandMarkProps) {
  if (src) {
    return (
      <img src={src} alt={title} className={cn('size-8 shrink-0 object-contain', className)} loading="lazy" />
    )
  }
  return (
    <svg
      viewBox="0 0 32 32"
      className={cn('size-8 shrink-0', className)}
      role="img"
      aria-label={title}
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <radialGradient id="polaris-brand-orb" cx="13" cy="11.5" r="18" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#FFD8A0" />
          <stop offset="0.3" stopColor="#FB923C" />
          <stop offset="0.6" stopColor="#7C5CFA" />
          <stop offset="0.84" stopColor="#5B8DEF" />
          <stop offset="1" stopColor="#22D3EE" />
        </radialGradient>
        <mask id="polaris-brand-star-core">
          <rect width="32" height="32" fill="#fff" />
          <circle cx="16" cy="16" r="2.5" fill="#000" />
        </mask>
      </defs>
      <circle cx="16" cy="16" r="16" fill="url(#polaris-brand-orb)" />
      <path
        d="M16 5.5 Q20 12 26.5 16 Q20 20 16 26.5 Q12 20 5.5 16 Q12 12 16 5.5Z"
        fill="#fff"
        mask="url(#polaris-brand-star-core)"
      />
    </svg>
  )
}
