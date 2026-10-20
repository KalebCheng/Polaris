import { QRCodeSVG } from 'qrcode.react'
import { cn } from '@/shared/lib/utils'

interface QrCodeProps {
  /** 待编码内容（如 otpauth:// 链接） */
  value: string
  /** 二维码边长（px） */
  size?: number
  className?: string
}

/**
 * 二维码（注册二次验证时给用户扫的那种）。
 *
 * 用 SVG 渲染：无论放大多少都清晰，且能被测试直接断言（无需 canvas）。
 * 仅做展示，语义交给相邻文字说明。
 */
export function QrCode({ value, size = 168, className }: QrCodeProps) {
  if (!value) return null
  return (
    <div className={cn('bg-white inline-flex rounded-xl border p-3', className)}>
      <QRCodeSVG value={value} size={size} level="M" marginSize={0} aria-hidden="true" />
    </div>
  )
}
