import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ArrowLeft, Mail, MessageSquare, Plus, Trash2 } from 'lucide-react'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
  Switch,
  Tabs,
  TabsList,
  TabsTrigger,
} from '@/shared/ui'
import { toast } from '@/shared/ui/toast'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { ProviderIcon } from '@/shared/components/provider-icon'
import { PageShell } from '@/widgets/layout/page-shell'
import { formatDateTime } from '@/shared/lib/format'
import { describeError } from '@/shared/lib/errors'
import type {
  Provider,
  ProviderTemplate,
  ProviderTemplateInput,
  ProviderTemplateParam,
  TemplateSceneInfo,
  TemplateVariable,
} from '@/shared/api/types'
import { providerApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { EmailRichTextEditor, type EmailRichTextEditorHandle } from '../components/email-rich-text-editor'

type TemplateScene = ProviderTemplate['scene']
type NotifyChannel = ProviderTemplate['channel']

interface TemplateDraft {
  name: string
  scene: TemplateScene
  templateCode: string
  templateParams: ProviderTemplateParam[]
  subject: string
  content: string
  isDefault: boolean
}

interface EditorState {
  template: ProviderTemplate | null
  scene: TemplateScene
}

const PREVIEW_VARS: Record<string, string> = {
  code: '123456',
  link: 'https://polaris.example.com/magic?token=preview',
  minutes: '5',
  phone: '138 0000 0000',
  institution: 'Acme',
  application: 'Polaris Console',
}

/**
 * 通知模板独立管理页：/institutions/:inst/providers/:id/templates
 *
 * 模板不再属于提供商「新增/编辑」流程：提供商只负责连接配置，模板按场景单独维护。
 * 场景与可用占位符来自后端 `GET /providers/message-template-scenes`，前端不另抄一份。
 */
export default function ProviderTemplatesPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const navigate = useNavigate()
  const { id = '' } = useParams()
  const providerID = Number(id)
  const [params, setParams] = useSearchParams()

  const [provider, setProvider] = useState<Provider | null>(null)
  const [scenes, setScenes] = useState<TemplateSceneInfo[]>([])
  const [templates, setTemplates] = useState<ProviderTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [deleting, setDeleting] = useState<ProviderTemplate | null>(null)
  const [deletingPending, setDeletingPending] = useState(false)

  const backToList = `/institutions/${institution}/providers`

  const reloadTemplates = useCallback(async () => {
    const data = await providerApi.templates(providerID)
    setTemplates(data.items ?? [])
  }, [providerID])

  useEffect(() => {
    if (!providerID || !institution) {
      setLoadFailed(true)
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    setLoadFailed(false)
    void Promise.all([
      providerApi.get(providerID, institution),
      providerApi.templateScenes(),
      providerApi.templates(providerID),
    ])
      .then(([providerData, sceneData, templateData]) => {
        if (cancelled) return
        setProvider(providerData)
        setScenes(sceneData.scenes ?? [])
        setTemplates(templateData.items ?? [])
      })
      .catch(() => {
        if (!cancelled) setLoadFailed(true)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [institution, providerID])

  const channel: NotifyChannel | null =
    provider?.category === 'email' || provider?.category === 'sms' ? provider.category : null
  const availableScenes = useMemo(() => scenes.filter((item) => item.channel === channel), [scenes, channel])
  const queryScene = params.get('scene') as TemplateScene | null
  const activeScene =
    availableScenes.find((item) => item.scene === queryScene)?.scene ??
    availableScenes[0]?.scene ??
    'login_code'
  const sceneInfo = availableScenes.find((item) => item.scene === activeScene)
  const sceneTemplates = templates.filter((item) => item.scene === activeScene)

  const changeScene = (value: string) => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.set('scene', value)
        return next
      },
      { replace: true },
    )
  }

  const openCreate = () => {
    setEditor({ template: null, scene: activeScene })
  }

  const openEdit = (template: ProviderTemplate) => {
    setEditor({ template, scene: template.scene })
  }

  const confirmDelete = async () => {
    if (!deleting) return
    setDeletingPending(true)
    try {
      await providerApi.removeTemplate(providerID, deleting.id)
      toast.success(t('providers.templates.deleted'))
      setDeleting(null)
      await reloadTemplates()
    } catch (error) {
      toast.error(describeError(error, t))
    } finally {
      setDeletingPending(false)
    }
  }

  if (!loading && (loadFailed || !provider || !channel || availableScenes.length === 0)) {
    return (
      <PageShell titleKey="providers.templates.pageTitle" subtitleKey="providers.templates.pageHint">
        <div className="surface space-y-3 rounded-2xl p-8 text-center">
          <p className="text-sm font-medium">{t('providers.notFound')}</p>
          <Button variant="outline" onClick={() => navigate(backToList)}>
            <ArrowLeft className="size-4" /> {t('common.back')}
          </Button>
        </div>
      </PageShell>
    )
  }

  return (
    <PageShell
      title={t('providers.templates.pageTitle')}
      description={
        provider
          ? t('providers.templates.pageHintForProvider', {
              provider: provider.display_name || provider.name,
            })
          : undefined
      }
      actions={
        <div className="flex items-center gap-2">
          <Button variant="linkMuted" size="sm" onClick={() => navigate(backToList)}>
            <ArrowLeft className="size-4" /> {t('providers.templates.backToProviders')}
          </Button>
          <Button onClick={openCreate} disabled={!sceneInfo}>
            <Plus className="size-4" /> {t('providers.templates.add')}
          </Button>
        </div>
      }
    >
      {loading ? (
        <div className="space-y-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-36 w-full" />
          <Skeleton className="h-36 w-full" />
        </div>
      ) : (
        <div className="space-y-5">
          {provider ? (
            <div className="surface flex items-center gap-3 rounded-2xl p-4">
              <div className="bg-muted flex size-10 shrink-0 items-center justify-center rounded-xl">
                <ProviderIcon iconKey={provider.type} size={20} />
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-semibold">{provider.display_name || provider.name}</p>
                <p className="text-muted-foreground truncate text-xs">
                  {t(`providers.category.${provider.category}`)} · {provider.name}
                </p>
              </div>
              <Badge variant="secondary">{t('providers.templates.count', { count: templates.length })}</Badge>
            </div>
          ) : null}

          <Tabs value={activeScene} onValueChange={changeScene}>
            <TabsList className="flex-wrap">
              {availableScenes.map((item) => (
                <TabsTrigger key={item.scene} value={item.scene}>
                  {sceneLabel(item, t)}
                  <span className="text-muted-foreground ml-1.5 text-[11px]">
                    {templates.filter((template) => template.scene === item.scene).length}
                  </span>
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>

          <div className="flex items-start gap-3">
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{sceneInfo ? sceneLabel(sceneInfo, t) : ''}</p>
              <p className="text-muted-foreground mt-1 text-xs">{t('providers.templates.sceneHint')}</p>
            </div>
            <div className="flex flex-wrap justify-end gap-1.5">
              {(sceneInfo?.variables ?? []).map((variable) => (
                <Badge key={variable.key} variant={variable.required ? 'default' : 'secondary'}>
                  {variableLabel(variable, t)}
                  {variable.required ? ` · ${t('providers.templates.required')}` : ''}
                </Badge>
              ))}
            </div>
          </div>

          {sceneTemplates.length === 0 ? (
            <div className="surface rounded-2xl p-8 text-center">
              <div className="bg-muted mx-auto flex size-12 items-center justify-center rounded-2xl">
                {channel === 'email' ? (
                  <Mail className="text-muted-foreground size-6" />
                ) : (
                  <MessageSquare className="text-muted-foreground size-6" />
                )}
              </div>
              <p className="mt-3 text-sm font-medium">{t('providers.templates.emptyScene')}</p>
              <p className="text-muted-foreground mt-1 text-xs">{t('providers.templates.emptySceneHint')}</p>
              <Button className="mt-4" variant="outline" onClick={openCreate} disabled={!sceneInfo}>
                <Plus className="size-4" /> {t('providers.templates.add')}
              </Button>
            </div>
          ) : (
            <div className="space-y-3">
              {sceneTemplates.map((template) => (
                <TemplateCard
                  key={template.id}
                  template={template}
                  onEdit={() => openEdit(template)}
                  onDelete={() => setDeleting(template)}
                />
              ))}
            </div>
          )}
        </div>
      )}

      {editor && provider && sceneInfo ? (
        <TemplateEditorDialog
          key={editor.template?.id ?? `new-${editor.scene}`}
          open
          provider={provider}
          sceneInfo={availableScenes.find((item) => item.scene === editor.scene) ?? sceneInfo}
          template={editor.template}
          onOpenChange={(open) => !open && setEditor(null)}
          onSaved={async () => {
            setEditor(null)
            await reloadTemplates().catch(() => undefined)
          }}
        />
      ) : null}

      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(open) => !open && setDeleting(null)}
        titleKey="providers.templates.deleteTitle"
        descriptionKey="providers.templates.deleteHint"
        loading={deletingPending}
        onConfirm={() => void confirmDelete()}
      />
    </PageShell>
  )
}

function TemplateCard({
  template,
  onEdit,
  onDelete,
}: {
  template: ProviderTemplate
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const isEmail = template.channel === 'email'
  return (
    <div className="surface rounded-2xl p-4">
      <div className="flex items-start gap-3">
        <div className="bg-muted flex size-10 shrink-0 items-center justify-center rounded-xl">
          {isEmail ? <Mail className="size-5" /> : <MessageSquare className="size-5" />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate text-sm font-semibold">{template.name}</p>
            {template.is_default ? (
              <Badge variant="secondary">{t('providers.templates.defaultBadge')}</Badge>
            ) : null}
            <Badge variant="outline">{sceneLabelByScene(template.scene, t)}</Badge>
          </div>
          <p className="text-muted-foreground mt-1.5 line-clamp-2 text-xs leading-relaxed">
            {templateSummary(template, t)}
          </p>
          <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px]">
            {isEmail ? null : (
              <span className="font-mono">{template.template_code || t('providers.templates.noCode')}</span>
            )}
            <span>{t('providers.templates.updatedAt', { time: formatDateTime(template.updated_at) })}</span>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button type="button" size="sm" variant="ghost" onClick={onEdit}>
            {t('common.edit')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="text-destructive hover:bg-destructive/10 hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>
    </div>
  )
}

function TemplateEditorDialog({
  open,
  provider,
  sceneInfo,
  template,
  onOpenChange,
  onSaved,
}: {
  open: boolean
  provider: Provider
  sceneInfo: TemplateSceneInfo
  template: ProviderTemplate | null
  onOpenChange: (open: boolean) => void
  onSaved: () => Promise<void>
}) {
  const { t } = useTranslation()
  const nameID = useId()
  const subjectID = useId()
  const codeID = useId()
  const bodyEditorRef = useRef<EmailRichTextEditorHandle>(null)
  const subjectRef = useRef<HTMLInputElement>(null)
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState<TemplateDraft>(() =>
    toDraft(template, provider.category, sceneInfo.scene),
  )

  const isEmail = sceneInfo.channel === 'email'
  const variables = sceneInfo.variables
  const required = variables.find((item) => item.required)

  const patch = (next: Partial<TemplateDraft>) => setDraft((prev) => ({ ...prev, ...next }))

  const insertSubjectPlaceholder = (variable: TemplateVariable) => {
    const token = `{{${variable.key}}}`
    const input = subjectRef.current
    if (!input) {
      patch({ subject: draft.subject + token })
      return
    }
    const start = input.selectionStart ?? input.value.length
    const end = input.selectionEnd ?? start
    const next = input.value.slice(0, start) + token + input.value.slice(end)
    patch({ subject: next })
    requestAnimationFrame(() => {
      input.focus()
      const position = start + token.length
      input.setSelectionRange(position, position)
    })
  }

  const validate = (): string | null => {
    if (!draft.name.trim()) return t('providers.templates.errName')
    if (isEmail) {
      if (!draft.content.trim()) return t('providers.templates.errContent')
      if (required && !hasPlaceholder(`${draft.subject}\n${draft.content}`, required.key)) {
        return t('providers.templates.errRequired', { placeholder: `{{${required.key}}}` })
      }
      return null
    }
    if (!draft.templateCode.trim()) return t('providers.templates.errTemplateCode')
    const params = normalizedParams(draft.templateParams)
    if (params.length === 0) return t('providers.templates.errParams')
    const seen = new Set<string>()
    for (const param of params) {
      if (seen.has(param.key)) return t('providers.templates.errDuplicateParam', { key: param.key })
      seen.add(param.key)
    }
    if (required && !params.some((param) => param.source === required.key)) {
      return t('providers.templates.errRequired', { placeholder: `{{${required.key}}}` })
    }
    return null
  }

  const save = async () => {
    const error = validate()
    if (error) {
      toast.error(error)
      return
    }
    const params = normalizedParams(draft.templateParams)
    const body: ProviderTemplateInput = {
      name: draft.name.trim(),
      scene: draft.scene,
      template_code: isEmail ? undefined : draft.templateCode.trim(),
      template_params: isEmail ? [] : params,
      subject: isEmail ? draft.subject.trim() : undefined,
      content: isEmail ? draft.content : undefined,
      is_default: draft.isDefault,
    }
    setSaving(true)
    try {
      if (template) await providerApi.updateTemplate(provider.id, template.id, body)
      else await providerApi.createTemplate(provider.id, body)
      toast.success(t('common.saved'))
      await onSaved()
    } catch (saveError) {
      toast.error(describeError(saveError, t))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>
            {template ? t('providers.templates.editTitle') : t('providers.templates.createTitle')}
          </DialogTitle>
          <DialogDescription>{sceneLabel(sceneInfo, t)}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_20rem]">
          <div className="min-w-0 space-y-4">
            <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
              <div className="space-y-1.5">
                <Label htmlFor={nameID}>{t('providers.templates.name')}</Label>
                <Input
                  id={nameID}
                  value={draft.name}
                  placeholder={t('providers.templates.namePlaceholder')}
                  onChange={(event) => patch({ name: event.target.value })}
                />
              </div>
              <label className="flex h-10 items-center gap-2 text-xs">
                <Switch
                  checked={draft.isDefault}
                  onCheckedChange={(checked) => patch({ isDefault: checked })}
                />
                {t('providers.templates.isDefault')}
              </label>
            </div>

            {isEmail ? (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor={subjectID}>{t('providers.templates.subject')}</Label>
                  <Input
                    id={subjectID}
                    ref={subjectRef}
                    value={draft.subject}
                    placeholder={t('providers.templates.subjectPlaceholder')}
                    onChange={(event) => patch({ subject: event.target.value })}
                  />
                  <PlaceholderPalette
                    variables={variables}
                    label={t('providers.templates.subjectPlaceholders')}
                    onInsert={insertSubjectPlaceholder}
                  />
                </div>
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between gap-3">
                    <Label>{t('providers.templates.content')}</Label>
                    <span className="text-muted-foreground text-[11px]">
                      {t('providers.templates.contentHint')}
                    </span>
                  </div>
                  <EmailRichTextEditor
                    ref={bodyEditorRef}
                    value={draft.content}
                    onChange={(content) => patch({ content })}
                    placeholder={t('providers.templates.contentPlaceholder')}
                  />
                </div>
              </>
            ) : (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor={codeID}>{t('providers.templates.code')}</Label>
                  <Input
                    id={codeID}
                    value={draft.templateCode}
                    placeholder={t('providers.templates.codePlaceholder')}
                    onChange={(event) => patch({ templateCode: event.target.value })}
                  />
                  <p className="text-muted-foreground text-[11px]">{t('providers.templates.codeHint')}</p>
                </div>
                <SmsParamEditor
                  variables={variables}
                  params={draft.templateParams}
                  onChange={(templateParams) => patch({ templateParams })}
                />
              </>
            )}
          </div>

          <aside className="min-w-0 space-y-4">
            {isEmail ? (
              <>
                <div className="surface space-y-2 rounded-2xl p-4">
                  <p className="text-[13px] font-semibold">{t('providers.templates.bodyPlaceholders')}</p>
                  <p className="text-muted-foreground text-[11px] leading-relaxed">
                    {t('providers.templates.bodyPlaceholdersHint')}
                  </p>
                  <PlaceholderPalette
                    variables={variables}
                    label=""
                    onInsert={(variable) => bodyEditorRef.current?.insertText(`{{${variable.key}}}`)}
                  />
                  {sceneInfo.scene === 'magic_link' ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="mt-1 w-full"
                      onClick={() =>
                        bodyEditorRef.current?.insertHTML(
                          `<a href="{{link}}" target="_blank" rel="noopener noreferrer">${t(
                            'providers.templates.magicLinkButton',
                          )}</a>`,
                        )
                      }
                    >
                      {t('providers.templates.insertMagicLink')}
                    </Button>
                  ) : null}
                </div>
                <EmailPreview subject={draft.subject} content={draft.content} />
              </>
            ) : (
              <SmsPreview
                templateCode={draft.templateCode}
                params={normalizedParams(draft.templateParams)}
                variables={variables}
              />
            )}
          </aside>
        </div>

        <DialogFooter className="border-border/70 border-t pt-4">
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button type="button" onClick={() => void save()} disabled={saving}>
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PlaceholderPalette({
  variables,
  label,
  onInsert,
}: {
  variables: TemplateVariable[]
  label: string
  onInsert: (variable: TemplateVariable) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-1.5">
      {label ? <p className="text-muted-foreground text-[11px]">{label}</p> : null}
      <div className="flex flex-wrap gap-1.5">
        {variables.map((variable) => (
          <Button
            key={variable.key}
            type="button"
            variant="outline"
            size="sm"
            className="h-7 px-2.5 text-[11px]"
            title={variableHint(variable, t)}
            onClick={() => onInsert(variable)}
          >
            {variableLabel(variable, t)}
            <span className="text-muted-foreground font-mono text-[10px]">{`{{${variable.key}}}`}</span>
          </Button>
        ))}
      </div>
    </div>
  )
}

function SmsParamEditor({
  variables,
  params,
  onChange,
}: {
  variables: TemplateVariable[]
  params: ProviderTemplateParam[]
  onChange: (next: ProviderTemplateParam[]) => void
}) {
  const { t } = useTranslation()
  const required = variables.find((item) => item.required)
  const add = () => {
    onChange([...params, { key: '', source: required?.key ?? variables[0]?.key ?? '' }])
  }
  const update = (index: number, next: Partial<ProviderTemplateParam>) => {
    onChange(
      params.map((item, current) => {
        if (current !== index) return item
        const updated = { ...item, ...next }
        if (next.source && !item.key) updated.key = next.source
        return updated
      }),
    )
  }
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-[13px] font-medium">{t('providers.templates.paramsTitle')}</p>
          <p className="text-muted-foreground mt-1 text-[11px] leading-relaxed">
            {t('providers.templates.paramsHint')}
          </p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={add}>
          <Plus className="size-4" /> {t('providers.templates.addParam')}
        </Button>
      </div>
      <div className="border-border/70 overflow-hidden rounded-xl border">
        <div className="bg-muted/40 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2.5rem] gap-2 px-3 py-2 text-[11px] font-medium">
          <span>{t('providers.templates.providerParam')}</span>
          <span>{t('providers.templates.platformPlaceholder')}</span>
          <span />
        </div>
        {params.length === 0 ? (
          <p className="text-muted-foreground px-3 py-4 text-center text-xs">
            {t('providers.templates.noParams')}
          </p>
        ) : (
          params.map((param, index) => (
            <div
              key={`${index}-${param.source}`}
              className="border-border/60 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2.5rem] items-center gap-2 border-t px-3 py-2"
            >
              <Input
                value={param.key}
                placeholder={t('providers.templates.providerParamPlaceholder')}
                onChange={(event) => update(index, { key: event.target.value })}
                className="h-9 font-mono text-xs"
              />
              <Select value={param.source} onValueChange={(value) => update(index, { source: value })}>
                <SelectTrigger className="h-9">
                  <SelectValue placeholder={t('providers.templates.choosePlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {variables.map((variable) => (
                    <SelectItem key={variable.key} value={variable.key}>
                      {variableLabel(variable, t)}（{variable.key}）
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8 text-muted-foreground"
                aria-label={t('common.delete')}
                onClick={() => onChange(params.filter((_, current) => current !== index))}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

function EmailPreview({ subject, content }: { subject: string; content: string }) {
  const { t } = useTranslation()
  const html = renderPreviewHTML(content)
  const renderedSubject = renderPreviewText(subject)
  return (
    <div className="surface space-y-2 rounded-2xl p-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-[13px] font-semibold">{t('providers.templates.preview')}</p>
        <Badge variant="secondary">{t('providers.templates.emailPreview')}</Badge>
      </div>
      <p className="text-muted-foreground truncate text-[11px]">
        {renderedSubject || t('providers.templates.noSubject')}
      </p>
      <iframe
        title={t('providers.templates.preview')}
        sandbox=""
        className="h-72 w-full rounded-xl border bg-white"
        srcDoc={html}
      />
    </div>
  )
}

function SmsPreview({
  templateCode,
  params,
  variables,
}: {
  templateCode: string
  params: ProviderTemplateParam[]
  variables: TemplateVariable[]
}) {
  const { t } = useTranslation()
  return (
    <div className="surface space-y-3 rounded-2xl p-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-[13px] font-semibold">{t('providers.templates.preview')}</p>
        <Badge variant="secondary">{t('providers.templates.smsPreview')}</Badge>
      </div>
      <div className="bg-muted/40 rounded-xl p-3">
        <p className="text-muted-foreground text-[10px] uppercase">
          {t('providers.templates.providerTemplate')}
        </p>
        <p className="mt-1 font-mono text-xs">{templateCode || '—'}</p>
      </div>
      <div className="space-y-1.5">
        <p className="text-muted-foreground text-[11px]">{t('providers.templates.sendValues')}</p>
        {params.length === 0 ? (
          <p className="text-muted-foreground text-xs">{t('providers.templates.noParams')}</p>
        ) : (
          params.map((param) => {
            const variable = variables.find((item) => item.key === param.source)
            return (
              <div
                key={`${param.key}-${param.source}`}
                className="flex items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-xs"
              >
                <span className="font-mono">{`\${${param.key}}`}</span>
                <span className="text-muted-foreground">
                  {variable ? variableLabel(variable, t) : param.source}
                  {PREVIEW_VARS[param.source] ? ` · ${PREVIEW_VARS[param.source]}` : ''}
                </span>
              </div>
            )
          })
        )}
      </div>
    </div>
  )
}

function sceneLabel(info: TemplateSceneInfo, t: (key: string, options?: Record<string, unknown>) => string) {
  const key = `providers.templates.scene.${info.scene}`
  const translated = t(key)
  return translated === key ? info.label : translated
}

function sceneLabelByScene(
  scene: TemplateScene,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  return sceneLabel(
    { channel: scene === 'magic_link' ? 'email' : 'sms', scene, label: scene, variables: [] },
    t,
  )
}

function templateSummary(
  template: ProviderTemplate,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (template.channel === 'email') {
    return template.subject || stripHTML(template.content) || t('providers.templates.noSubject')
  }
  const params = template.template_params ?? []
  if (params.length === 0) return t('providers.templates.noParams')
  return params.map((param) => `${param.key} ← ${param.source}`).join(' · ')
}

function toDraft(template: ProviderTemplate | null, category: string, scene: TemplateScene): TemplateDraft {
  if (template) {
    return {
      name: template.name,
      scene: template.scene,
      templateCode: template.template_code,
      templateParams: (template.template_params ?? []).map((item) => ({ ...item })),
      subject: template.subject,
      content: template.content,
      isDefault: template.is_default,
    }
  }
  const sms = category === 'sms'
  return {
    name: '',
    scene,
    templateCode: '',
    templateParams: sms ? [{ key: 'code', source: 'code' }] : [],
    subject: '',
    content: '',
    isDefault: false,
  }
}

function normalizedParams(params: ProviderTemplateParam[]): ProviderTemplateParam[] {
  return params
    .map((item) => ({
      key: item.key.trim() || item.source,
      source: item.source,
    }))
    .filter((item) => item.source)
}

function hasPlaceholder(text: string, key: string): boolean {
  return new RegExp(`\\{\\{\\s*${key}\\s*\\}\\}`).test(text)
}

function renderPreviewText(text: string): string {
  let out = text
  for (const [key, value] of Object.entries(PREVIEW_VARS)) {
    out = out.replaceAll(`{{${key}}}`, value)
  }
  return out
}

function renderPreviewHTML(content: string): string {
  const body = renderPreviewText(content)
  return `<!doctype html><html><head><meta charset="utf-8" /><style>
    body { margin: 20px; color: #111827; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; font-size: 14px; line-height: 1.65; }
    h1 { font-size: 22px; } h2 { font-size: 18px; } h3 { font-size: 16px; }
    a { color: #2563eb; } blockquote { margin: 12px 0; padding-left: 12px; border-left: 3px solid #bfdbfe; color: #4b5563; }
    ul, ol { padding-left: 20px; } hr { border: 0; border-top: 1px solid #e5e7eb; }
  </style></head><body>${body || '<p style="color:#9ca3af">—</p>'}</body></html>`
}

function stripHTML(html: string): string {
  return html
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function variableLabel(variable: TemplateVariable, t: (key: string) => string): string {
  const key = `providers.templates.var.${variable.key}`
  const translated = t(key)
  return translated === key ? variable.label : translated
}

function variableHint(variable: TemplateVariable, t: (key: string) => string): string {
  const key = `providers.templates.varHint.${variable.key}`
  const translated = t(key)
  return translated === key ? (variable.hint ?? '') : translated
}
