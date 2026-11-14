import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, KeyRound, Pencil, Plus, Power, Trash2 } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Input,
  MultiSelect,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
  Switch,
} from '@/shared/ui'
import { toast } from '@/shared/ui/toast'
import { PageShell } from '@/widgets/layout/page-shell'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { Field } from '../components/field'
import { providerApi, signinMethodApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import type {
  InstitutionSigninMethod,
  InstitutionSigninMethodInput,
  Provider,
  ProviderTemplate,
} from '@/shared/api/types'

const KINDS = ['Password', 'EmailCode', 'SMSCode', 'MagicLink', 'Provider'] as const
type Kind = (typeof KINDS)[number]

const kindLabelKey: Record<Kind, string> = {
  Password: 'signinMethods.kindPassword',
  EmailCode: 'signinMethods.kindEmailCode',
  SMSCode: 'signinMethods.kindSMSCode',
  MagicLink: 'signinMethods.kindMagicLink',
  Provider: 'signinMethods.kindProvider',
}

const identifierOptions = [
  { value: 'username', label: 'username' },
  { value: 'email', label: 'email' },
  { value: 'phone', label: 'phone' },
]

interface FormState {
  name: Kind
  display_name: string
  identifiers: string[]
  provider_id: string
  template_id: string
  ttl_seconds: number
  mfa_enabled: boolean
  mfa_required: boolean
  mfa_provider_ids: string[]
  sort_order: number
}

const emptyForm: FormState = {
  name: 'Password',
  display_name: '',
  identifiers: ['username', 'email'],
  provider_id: '',
  template_id: '',
  ttl_seconds: 300,
  mfa_enabled: true,
  mfa_required: false,
  mfa_provider_ids: [],
  sort_order: 10,
}

function categoryOf(value?: string) {
  return (value ?? '').toLowerCase()
}

export default function SigninMethodsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const institution = useWorkspaceInstitution()
  const institutionID = Number(useWorkspaceInstitutionId())

  const [loading, setLoading] = useState(true)
  const [methods, setMethods] = useState<InstitutionSigninMethod[]>([])
  const [providers, setProviders] = useState<Provider[]>([])
  const [templates, setTemplates] = useState<ProviderTemplate[]>([])
  const [editing, setEditing] = useState<InstitutionSigninMethod | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [form, setForm] = useState<FormState>(emptyForm)
  const [pending, setPending] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<InstitutionSigninMethod | null>(null)

  const load = useCallback(async () => {
    if (!institutionID) return
    setLoading(true)
    try {
      const [methodList, providerList] = await Promise.all([
        signinMethodApi.list(institutionID),
        providerApi.list({ institution, size: 200 }).catch(() => ({ items: [] as Provider[] })),
      ])
      setMethods(methodList.items ?? [])
      setProviders(providerList.items ?? [])
    } catch {
      toast.error(t('common.loadFailed'))
    } finally {
      setLoading(false)
    }
  }, [institution, institutionID, t])

  useEffect(() => {
    void load()
  }, [load])

  const emailProviders = useMemo(
    () => providers.filter((item) => categoryOf(item.category) === 'email' && item.state === 'active'),
    [providers],
  )
  const smsProviders = useMemo(
    () => providers.filter((item) => categoryOf(item.category) === 'sms' && item.state === 'active'),
    [providers],
  )
  const oauthProviders = useMemo(
    () => providers.filter((item) => categoryOf(item.category) === 'oauth' && item.state === 'active'),
    [providers],
  )
  const mfaProviders = useMemo(
    () => providers.filter((item) => categoryOf(item.category) === 'mfa' && item.state === 'active'),
    [providers],
  )

  const providerOptions = useMemo(() => {
    if (form.name === 'Provider') return oauthProviders
    if (form.name === 'SMSCode') return smsProviders
    return emailProviders
  }, [form.name, oauthProviders, emailProviders, smsProviders])

  const selectedProvider = providers.find((item) => String(item.id) === form.provider_id)
  const selectedProviderID = selectedProvider?.id ?? 0
  const templateScene =
    form.name === 'MagicLink' ? 'magic_link' : form.name === 'SMSCode' ? 'login_code' : 'login_code'

  useEffect(() => {
    if (!dialogOpen || !selectedProviderID || form.name === 'Password' || form.name === 'Provider') {
      setTemplates([])
      return
    }
    let cancelled = false
    void providerApi
      .templates(selectedProviderID, templateScene, institution)
      .then((result) => {
        if (!cancelled) setTemplates(result.items ?? [])
      })
      .catch(() => {
        if (!cancelled) setTemplates([])
      })
    return () => {
      cancelled = true
    }
  }, [dialogOpen, selectedProviderID, form.name, templateScene, institution])

  const openCreate = () => {
    setEditing(null)
    setForm({
      ...emptyForm,
      sort_order: (methods.reduce((max, item) => Math.max(max, item.sort_order), 0) || 0) + 10,
    })
    setDialogOpen(true)
  }

  const openEdit = (method: InstitutionSigninMethod) => {
    setEditing(method)
    setForm({
      name: method.name as Kind,
      display_name: method.display_name,
      identifiers: method.identifiers ?? [],
      provider_id: method.provider_id ? String(method.provider_id) : '',
      template_id: String(method.email_template_id ?? method.sms_template_id ?? ''),
      ttl_seconds: method.ttl_seconds,
      mfa_enabled: method.mfa_enabled,
      mfa_required: method.mfa_required,
      mfa_provider_ids: (method.mfa_provider_ids ?? []).map(String),
      sort_order: method.sort_order,
    })
    setDialogOpen(true)
  }

  const save = async () => {
    if (!form.name) return
    if (form.mfa_required && !form.mfa_enabled) {
      toast.error(t('signinMethods.mfaRequiredNeedsEnabled'))
      return
    }
    if (
      (form.name === 'EmailCode' || form.name === 'SMSCode' || form.name === 'MagicLink') &&
      (!form.provider_id || !form.template_id)
    ) {
      toast.error(t('signinMethods.channelRequired'))
      return
    }
    if (form.name === 'Provider' && !form.provider_id) {
      toast.error(t('signinMethods.providerRequired'))
      return
    }
    if (form.name === 'Password' && form.identifiers.length === 0) {
      toast.error(t('signinMethods.identifierRequired'))
      return
    }
    const input: InstitutionSigninMethodInput = {
      name: form.name,
      display_name: form.display_name.trim(),
      identifiers: form.name === 'Password' ? form.identifiers : [],
      provider_id: form.provider_id ? Number(form.provider_id) : null,
      email_template_id:
        form.name === 'EmailCode' || form.name === 'MagicLink' ? Number(form.template_id) || null : null,
      sms_template_id: form.name === 'SMSCode' ? Number(form.template_id) || null : null,
      ttl_seconds:
        form.name === 'EmailCode' || form.name === 'SMSCode' || form.name === 'MagicLink'
          ? Number(form.ttl_seconds) || 300
          : 0,
      mfa_enabled: form.mfa_enabled,
      mfa_required: form.mfa_enabled && form.mfa_required,
      mfa_provider_ids: form.mfa_provider_ids.map(Number),
      sort_order: Number(form.sort_order) || 0,
    }
    setPending(true)
    try {
      if (editing) await signinMethodApi.update(institutionID, editing.id, input)
      else await signinMethodApi.create(institutionID, input)
      toast.success(t('common.saved'))
      setDialogOpen(false)
      await load()
    } catch {
      toast.error(t('common.operationFailed'))
    } finally {
      setPending(false)
    }
  }

  const toggleState = async (method: InstitutionSigninMethod) => {
    try {
      await signinMethodApi.setState(
        institutionID,
        method.id,
        method.state === 'active' ? 'disabled' : 'active',
      )
      await load()
    } catch {
      toast.error(t('signinMethods.stateConflict'))
    }
  }

  const remove = async () => {
    if (!deleteTarget) return
    setPending(true)
    try {
      await signinMethodApi.remove(institutionID, deleteTarget.id)
      toast.success(t('common.deleted'))
      setDeleteTarget(null)
      await load()
    } catch {
      toast.error(t('signinMethods.deleteConflict'))
    } finally {
      setPending(false)
    }
  }

  return (
    <PageShell
      title={t('signinMethods.title')}
      description={t('signinMethods.subtitle')}
      actions={
        <>
          <Button variant="outline" onClick={() => navigate(`/institutions/${institutionID}`)}>
            <ArrowLeft className="size-4" /> {t('signinMethods.backToOrg')}
          </Button>
          <Button onClick={openCreate}>
            <Plus className="size-4" /> {t('signinMethods.create')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div className="grid gap-4 md:grid-cols-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-40 w-full" />
          ))}
        </div>
      ) : methods.length === 0 ? (
        <Card>
          <CardContent className="text-muted-foreground py-12 text-center text-sm">
            {t('signinMethods.empty')}
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {methods.map((method) => (
            <Card key={method.id}>
              <CardContent className="space-y-3 py-5">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-xl">
                      <KeyRound className="size-4" />
                    </span>
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold">{method.display_name || method.name}</p>
                      <p className="text-muted-foreground mt-0.5 text-xs">
                        {t(kindLabelKey[method.name as Kind] ?? 'signinMethods.kindPassword')}
                      </p>
                    </div>
                  </div>
                  <Badge variant={method.state === 'active' ? 'success' : 'destructive'}>
                    {method.state === 'active' ? t('common.enabled') : t('common.disabled')}
                  </Badge>
                </div>

                <div className="flex flex-wrap gap-1.5">
                  {method.provider_name ? <Badge variant="outline">{method.provider_name}</Badge> : null}
                  {method.template_name ? <Badge variant="outline">{method.template_name}</Badge> : null}
                  {method.ttl_seconds ? (
                    <Badge variant="secondary">
                      {t('signinMethods.ttlBadge', { seconds: method.ttl_seconds })}
                    </Badge>
                  ) : null}
                  {method.mfa_enabled ? (
                    <Badge variant="secondary">
                      {method.mfa_required
                        ? t('applications.mfaRequiredBadge')
                        : t('applications.mfaEnabledBadge')}
                    </Badge>
                  ) : null}
                  <Badge variant="secondary">
                    {t('signinMethods.applicationCount', { count: method.application_count })}
                  </Badge>
                </div>

                {method.provider_state === 'disabled' ? (
                  <p className="text-destructive text-xs">{t('signinMethods.providerDisabled')}</p>
                ) : null}

                <div className="flex flex-wrap gap-2 pt-1">
                  <Button variant="outline" size="sm" onClick={() => openEdit(method)}>
                    <Pencil className="size-4" /> {t('common.edit')}
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => void toggleState(method)}>
                    <Power className="size-4" /> {t('signinMethods.toggle')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={method.application_count > 0}
                    onClick={() => setDeleteTarget(method)}
                  >
                    <Trash2 className="size-4" /> {t('common.delete')}
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {editing ? t('signinMethods.editTitle') : t('signinMethods.createTitle')}
            </DialogTitle>
            <DialogDescription>{t('signinMethods.dialogHint')}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="method-kind" labelKey="signinMethods.kind">
              <Select
                value={form.name}
                disabled={Boolean(editing)}
                onValueChange={(value) =>
                  setForm((prev) => ({
                    ...prev,
                    name: value as Kind,
                    provider_id: '',
                    template_id: '',
                    identifiers: value === 'Password' ? ['username', 'email'] : [],
                    ttl_seconds: value === 'MagicLink' ? 600 : 300,
                  }))
                }
              >
                <SelectTrigger id="method-kind">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {KINDS.map((kind) => (
                    <SelectItem key={kind} value={kind}>
                      {t(kindLabelKey[kind])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field id="method-display" labelKey="signinMethods.displayName">
              <Input
                id="method-display"
                value={form.display_name}
                onChange={(event) => setForm((prev) => ({ ...prev, display_name: event.target.value }))}
              />
            </Field>

            {form.name === 'Password' ? (
              <Field id="method-identifiers" labelKey="signinMethods.identifiers" className="sm:col-span-2">
                <MultiSelect
                  options={identifierOptions}
                  value={form.identifiers}
                  onChange={(value) => setForm((prev) => ({ ...prev, identifiers: value }))}
                  placeholder={t('common.selectPlaceholder')}
                />
              </Field>
            ) : null}

            {form.name !== 'Password' ? (
              <Field id="method-provider" labelKey="signinMethods.provider">
                <Select
                  value={form.provider_id}
                  disabled={Boolean(editing) && form.name === 'Provider'}
                  onValueChange={(value) =>
                    setForm((prev) => ({ ...prev, provider_id: value, template_id: '' }))
                  }
                >
                  <SelectTrigger id="method-provider">
                    <SelectValue placeholder={t('common.selectPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {providerOptions.map((provider) => (
                      <SelectItem key={provider.id} value={String(provider.id)}>
                        {provider.display_name || provider.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            ) : null}

            {form.name === 'EmailCode' || form.name === 'SMSCode' || form.name === 'MagicLink' ? (
              <>
                <Field id="method-template" labelKey="signinMethods.template">
                  <Select
                    value={form.template_id}
                    onValueChange={(value) => setForm((prev) => ({ ...prev, template_id: value }))}
                  >
                    <SelectTrigger id="method-template">
                      <SelectValue placeholder={t('common.selectPlaceholder')} />
                    </SelectTrigger>
                    <SelectContent>
                      {templates.map((template) => (
                        <SelectItem key={template.id} value={String(template.id)}>
                          {template.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field id="method-ttl" labelKey="signinMethods.ttl">
                  <Input
                    id="method-ttl"
                    type="number"
                    min={30}
                    value={form.ttl_seconds}
                    onChange={(event) =>
                      setForm((prev) => ({ ...prev, ttl_seconds: Number(event.target.value) }))
                    }
                  />
                </Field>
              </>
            ) : null}

            <Field id="method-sort" labelKey="signinMethods.sortOrder">
              <Input
                id="method-sort"
                type="number"
                min={0}
                value={form.sort_order}
                onChange={(event) => setForm((prev) => ({ ...prev, sort_order: Number(event.target.value) }))}
              />
            </Field>

            <div className="border-border/70 space-y-3 rounded-xl border p-3 sm:col-span-2">
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="flex items-center justify-between gap-3 text-sm">
                  <span>{t('applications.mfaEnabled')}</span>
                  <Switch
                    checked={form.mfa_enabled}
                    onCheckedChange={(checked) =>
                      setForm((prev) => ({
                        ...prev,
                        mfa_enabled: checked,
                        mfa_required: checked ? prev.mfa_required : false,
                      }))
                    }
                  />
                </label>
                <label className="flex items-center justify-between gap-3 text-sm">
                  <span>{t('applications.mfaRequired')}</span>
                  <Switch
                    checked={form.mfa_required}
                    disabled={!form.mfa_enabled}
                    onCheckedChange={(checked) => setForm((prev) => ({ ...prev, mfa_required: checked }))}
                  />
                </label>
              </div>
              {form.mfa_enabled ? (
                <Field id="method-mfa-providers" labelKey="applications.mfaProviders">
                  <MultiSelect
                    options={mfaProviders.map((provider) => ({
                      value: String(provider.id),
                      label: provider.display_name || provider.name,
                    }))}
                    value={form.mfa_provider_ids}
                    onChange={(value) => setForm((prev) => ({ ...prev, mfa_provider_ids: value }))}
                    placeholder={t('common.selectPlaceholder')}
                  />
                </Field>
              ) : null}
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={pending} onClick={() => void save()}>
              {t('common.save')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        titleKey="signinMethods.deleteTitle"
        descriptionKey="signinMethods.deleteBody"
        confirmLabelKey="common.delete"
        loading={pending}
        onConfirm={() => void remove()}
      />
    </PageShell>
  )
}
