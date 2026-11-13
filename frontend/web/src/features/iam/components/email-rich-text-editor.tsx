import { forwardRef, useEffect, useImperativeHandle } from 'react'
import { EditorContent, useEditor, useEditorState } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { useTranslation } from 'react-i18next'
import {
  Bold,
  Italic,
  List as ListIcon,
  ListOrdered,
  Minus,
  Quote,
  Redo2,
  Strikethrough,
  Underline as UnderlineIcon,
  Undo2,
} from 'lucide-react'
import { Button, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui'
import { cn } from '@/shared/lib/utils'

interface EmailRichTextEditorProps {
  value: string
  onChange: (html: string) => void
  placeholder?: string
}

export interface EmailRichTextEditorHandle {
  insertText: (text: string) => void
  insertHTML: (html: string) => void
}

const EMPTY_DOCUMENT = '<p></p>'

/** 邮件正文富文本编辑器：标签与属性会被后端收敛到邮件安全集合。 */
export const EmailRichTextEditor = forwardRef<EmailRichTextEditorHandle, EmailRichTextEditorProps>(
  function EmailRichTextEditor({ value, onChange, placeholder }, ref) {
    const { t } = useTranslation()
    const editor = useEditor({
      immediatelyRender: false,
      extensions: [
        StarterKit.configure({
          code: false,
          codeBlock: false,
          heading: { levels: [1, 2, 3] },
          link: {
            openOnClick: false,
            autolink: true,
            linkOnPaste: true,
            // 魔法链接按占位符落库（{{link}}），默认 URL 校验会把它当作非法协议；
            // 最终安全边界在后端清洗，这里只放行占位符本身。
            isAllowedUri: (url, { defaultValidate }) =>
              /^\{\{[a-z0-9_]+\}\}$/.test(url.trim()) || defaultValidate(url),
            HTMLAttributes: { rel: 'noopener noreferrer', target: '_blank' },
          },
        }),
      ],
      content: value || EMPTY_DOCUMENT,
      editorProps: {
        attributes: {
          class:
            'min-h-48 px-4 py-3 text-[13px] leading-relaxed outline-none ' +
            '[&_h1]:mt-3 [&_h1]:mb-2 [&_h1]:text-xl [&_h1]:font-semibold ' +
            '[&_h2]:mt-3 [&_h2]:mb-2 [&_h2]:text-lg [&_h2]:font-semibold ' +
            '[&_h3]:mt-3 [&_h3]:mb-2 [&_h3]:text-base [&_h3]:font-semibold ' +
            '[&_p]:my-2 [&_ul]:my-2 [&_ul]:list-disc [&_ul]:pl-5 ' +
            '[&_ol]:my-2 [&_ol]:list-decimal [&_ol]:pl-5 ' +
            '[&_blockquote]:border-primary/40 [&_blockquote]:my-3 [&_blockquote]:border-l-2 [&_blockquote]:pl-3 ' +
            '[&_a]:text-primary [&_a]:underline [&_hr]:my-4 [&_hr]:border-border',
        },
      },
      onUpdate: ({ editor: current }) => onChange(current.isEmpty ? '' : current.getHTML()),
    })

    useEffect(() => {
      if (!editor) return
      const current = editor.isEmpty ? '' : editor.getHTML()
      if (current === value) return
      editor.commands.setContent(value || EMPTY_DOCUMENT, { emitUpdate: false })
    }, [editor, value])

    const state = useEditorState({
      editor,
      selector: ({ editor: current }) => {
        if (!current) {
          return {
            bold: false,
            italic: false,
            underline: false,
            strike: false,
            bulletList: false,
            orderedList: false,
            blockquote: false,
            block: 'paragraph',
            canUndo: false,
            canRedo: false,
          }
        }
        return {
          bold: current.isActive('bold'),
          italic: current.isActive('italic'),
          underline: current.isActive('underline'),
          strike: current.isActive('strike'),
          bulletList: current.isActive('bulletList'),
          orderedList: current.isActive('orderedList'),
          blockquote: current.isActive('blockquote'),
          block: current.isActive('heading', { level: 1 })
            ? 'h1'
            : current.isActive('heading', { level: 2 })
              ? 'h2'
              : current.isActive('heading', { level: 3 })
                ? 'h3'
                : 'paragraph',
          canUndo: current.can().undo(),
          canRedo: current.can().redo(),
        }
      },
    })

    useImperativeHandle(
      ref,
      () => ({
        insertText: (text: string) => {
          editor?.chain().focus().insertContent({ type: 'text', text }).run()
        },
        insertHTML: (html: string) => {
          editor?.chain().focus().insertContent(html).run()
        },
      }),
      [editor],
    )

    if (!editor) {
      return <div className="border-input/70 bg-card min-h-60 rounded-xl border" />
    }

    const run = (action: () => void) => {
      action()
      editor.chain().focus().run()
    }

    return (
      <div className="border-input/70 bg-card overflow-hidden rounded-xl border">
        <div className="border-border/70 flex flex-wrap items-center gap-0.5 border-b p-1.5">
          <Select
            value={state?.block ?? 'paragraph'}
            onValueChange={(next) => {
              const chain = editor.chain().focus()
              if (next === 'paragraph') chain.setParagraph().run()
              else chain.toggleHeading({ level: Number(next.slice(1)) as 1 | 2 | 3 }).run()
            }}
          >
            <SelectTrigger className="h-8 w-28 rounded-lg border-0 bg-transparent px-2 shadow-none">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="paragraph">{t('providers.templates.editor.paragraph')}</SelectItem>
              <SelectItem value="h1">{t('providers.templates.editor.heading1')}</SelectItem>
              <SelectItem value="h2">{t('providers.templates.editor.heading2')}</SelectItem>
              <SelectItem value="h3">{t('providers.templates.editor.heading3')}</SelectItem>
            </SelectContent>
          </Select>
          <ToolbarButton
            label={t('providers.templates.editor.bold')}
            active={state?.bold}
            onClick={() => run(() => editor.chain().toggleBold().run())}
          >
            <Bold className="size-4" />
          </ToolbarButton>
          <ToolbarButton
            label={t('providers.templates.editor.italic')}
            active={state?.italic}
            onClick={() => run(() => editor.chain().toggleItalic().run())}
          >
            <Italic className="size-4" />
          </ToolbarButton>
          <ToolbarButton
            label={t('providers.templates.editor.underline')}
            active={state?.underline}
            onClick={() => run(() => editor.chain().toggleUnderline().run())}
          >
            <UnderlineIcon className="size-4" />
          </ToolbarButton>
          <ToolbarButton
            label={t('providers.templates.editor.strike')}
            active={state?.strike}
            onClick={() => run(() => editor.chain().toggleStrike().run())}
          >
            <Strikethrough className="size-4" />
          </ToolbarButton>
          <span className="bg-border/70 mx-1 h-5 w-px" />
          <ToolbarButton
            label={t('providers.templates.editor.bulletList')}
            active={state?.bulletList}
            onClick={() => run(() => editor.chain().toggleBulletList().run())}
          >
            <ListIcon className="size-4" />
          </ToolbarButton>
          <ToolbarButton
            label={t('providers.templates.editor.orderedList')}
            active={state?.orderedList}
            onClick={() => run(() => editor.chain().toggleOrderedList().run())}
          >
            <ListOrdered className="size-4" />
          </ToolbarButton>
          <ToolbarButton
            label={t('providers.templates.editor.blockquote')}
            active={state?.blockquote}
            onClick={() => run(() => editor.chain().toggleBlockquote().run())}
          >
            <Quote className="size-4" />
          </ToolbarButton>
          <ToolbarButton
            label={t('providers.templates.editor.horizontalRule')}
            onClick={() => run(() => editor.chain().setHorizontalRule().run())}
          >
            <Minus className="size-4" />
          </ToolbarButton>
          <span className="bg-border/70 mx-1 h-5 w-px" />
          <ToolbarButton
            label={t('providers.templates.editor.undo')}
            disabled={!state?.canUndo}
            onClick={() => run(() => editor.chain().undo().run())}
          >
            <Undo2 className="size-4" />
          </ToolbarButton>
          <ToolbarButton
            label={t('providers.templates.editor.redo')}
            disabled={!state?.canRedo}
            onClick={() => run(() => editor.chain().redo().run())}
          >
            <Redo2 className="size-4" />
          </ToolbarButton>
        </div>
        <div className="relative">
          <EditorContent editor={editor} />
          {editor.isEmpty && placeholder ? (
            <span className="text-muted-foreground pointer-events-none absolute top-3 left-4 text-[13px]">
              {placeholder}
            </span>
          ) : null}
        </div>
      </div>
    )
  },
)

function ToolbarButton({
  label,
  active,
  disabled,
  onClick,
  children,
}: {
  label: string
  active?: boolean
  disabled?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      title={label}
      aria-label={label}
      aria-pressed={active}
      disabled={disabled}
      onClick={onClick}
      className={cn('size-8 rounded-lg', active && 'bg-accent text-accent-foreground')}
    >
      {children}
    </Button>
  )
}
