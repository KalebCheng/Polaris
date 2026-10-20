import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ImageUp, Loader2, Upload, X } from 'lucide-react'
import { api } from '@/shared/api/client'
import { describeError } from '@/shared/lib/errors'
import { cn } from '@/shared/lib/utils'
import { Button, Input } from '@/shared/ui'

interface UploadOrURLProps {
  id?: string
  value: string
  onChange: (value: string) => void
  /** 上传分类（后端白名单：logos / favicons / avatars / application / images） */
  category?: string
  /** 接受的文件类型（默认图片） */
  accept?: string
  placeholder?: string
  disabled?: boolean
  /** 是否显示缩略图预览（默认显示） */
  preview?: boolean
  /** 当前工作区组织（组织主键 id）：上传要落到该组织的存储提供商，不能落到会话所属组织 */
  institution?: string
  className?: string
}

const isImage = (value: string) => /\.(png|jpe?g|gif|webp|svg)(\?.*)?$/i.test(value)

/**
 * 「上传或填 URL」二合一字段（组织 Icon / Favicon 等）。
 *
 * 交互取向：**单行紧凑**——左侧缩略图（点击选文件、可拖拽投放），中间直接填写或粘贴地址，
 * 右侧一个「上传」按钮。不做模式切换、不做大号投放区，避免撑高表单行、造成布局跳动。
 */
export function UploadOrURL({
  id,
  value,
  onChange,
  category = 'logos',
  accept = 'image/png,image/jpeg,image/gif,image/webp,image/svg+xml',
  placeholder = 'https://example.com/logo.png',
  disabled,
  preview = true,
  institution,
  className,
}: UploadOrURLProps) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const [dragging, setDragging] = useState(false)

  const upload = async (file: File) => {
    setUploading(true)
    try {
      const form = new FormData()
      form.append('file', file)
      form.append('category', category)
      // 上传写入的是**当前工作区组织**的存储提供商（后端按 ?institution= 解析）
      const res = await api.upload<{ url: string }>('/upload', form, { institution })
      onChange(res.url)
      toast.success(t('upload.success'))
    } catch (error) {
      toast.error(describeError(error, t))
    } finally {
      setUploading(false)
    }
  }

  const pick = (file: File | undefined | null) => {
    if (file) void upload(file)
  }

  return (
    <div className={cn('flex items-center gap-2.5', className)}>
      {preview ? (
        <button
          type="button"
          disabled={disabled || uploading}
          aria-label={t('upload.chooseFile')}
          title={t('upload.chooseFile')}
          onClick={() => inputRef.current?.click()}
          onDragOver={(event) => {
            event.preventDefault()
            if (!disabled) setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(event) => {
            event.preventDefault()
            setDragging(false)
            if (disabled) return
            pick(event.dataTransfer.files?.[0])
          }}
          className={cn(
            'bg-muted/40 relative flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-md border transition-colors',
            dragging ? 'border-primary ring-primary/30 ring-2' : 'hover:border-primary/60',
            (disabled || uploading) && 'cursor-not-allowed opacity-70',
          )}
        >
          {value && isImage(value) ? (
            <img src={value} alt="" className="size-full object-contain" />
          ) : (
            <ImageUp className="text-muted-foreground size-4" aria-hidden="true" />
          )}
          {uploading ? (
            <span className="bg-background/60 absolute inset-0 flex items-center justify-center">
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            </span>
          ) : null}
        </button>
      ) : null}

      <Input
        id={id}
        value={value}
        disabled={disabled}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        className="min-w-0 flex-1"
      />

      <Button
        type="button"
        variant="outline"
        size="sm"
        className="shrink-0 gap-1.5"
        disabled={disabled || uploading}
        onClick={() => inputRef.current?.click()}
      >
        {uploading ? <Loader2 className="size-3.5 animate-spin" /> : <Upload className="size-3.5" />}
        {t('upload.upload')}
      </Button>

      {value ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-9 shrink-0"
          aria-label={t('upload.clear')}
          title={t('upload.clear')}
          onClick={() => onChange('')}
        >
          <X className="size-4" />
        </Button>
      ) : null}

      <input
        ref={inputRef}
        type="file"
        accept={accept}
        className="hidden"
        onChange={(event) => {
          pick(event.target.files?.[0])
          event.target.value = ''
        }}
      />
    </div>
  )
}
