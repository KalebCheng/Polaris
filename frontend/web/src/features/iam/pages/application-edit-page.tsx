import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowLeft, CheckCircle2, ExternalLink, Eye, Plus } from 'lucide-react'
import {
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Skeleton,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { CopyButton } from '@/shared/components/copy-button'
import type { Application, ApplicationCreated, Cert, Group, Provider } from '@/shared/api/types'
import { applicationApi, certApi, groupApi, providerApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useMutation } from '../hooks/use-mutation'
import { buildAuthorizeUrl } from '../lib/authorize-url'
import { ApplicationWizard } from '../components/application-wizard'
import { LoginPagePreview } from '../components/login-page-preview'
import {
  applicationToForm,
  emptyApplicationForm,
  formToApplication,
  formToApplicationValidation,
  signinMethodKinds,
  type ApplicationFormValue,
} from '../components/application-form'

/**
 * 应用新建 / 编辑页。
 * 路由：/institutions/:inst/applications/new 与 /institutions/:inst/applications/:code
 *
 * 创建与编辑共用同一「四步向导」；登录页预览是**按需弹窗**（对齐组织创建/编辑），
 * 不再占用右侧常驻栏；创建成功后停留在完成页，一次性展示客户端凭据与端点清单。
 */
export default function ApplicationEditPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const navigate = useNavigate()
  const { id = '' } = useParams()
  const appID = Number(id)
  const isCreate = !id || id === 'new'

  const { pending, run } = useMutation()
  const [form, setForm] = useState<ApplicationFormValue>(() => ({ ...emptyApplicationForm, institution_id: Number(institution) }))
  const [editing, setEditing] = useState<Application | null>(null)
  const [created, setCreated] = useState<ApplicationCreated | null>(null)
  const [loading, setLoading] = useState(!isCreate)
  const [confirmDiscard, setConfirmDiscard] = useState(false)
  const [providers, setProviders] = useState<Provider[]>([])
  const [certs, setCerts] = useState<Cert[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewMode, setPreviewMode] = useState<'signin' | 'signup'>('signin')

  // 脏值基线：用于「未保存修改」拦截
  const baseline = useRef<string>(JSON.stringify({ ...emptyApplicationForm, institution_id: Number(institution) }))
  const dirty = !created && JSON.stringify(form) !== baseline.current

  // 可选资源来自当前组织，供下拉选择使用
  useEffect(() => {
    if (!institution) return
    let cancelled = false
    void (async () => {
      const [providerList, certList, groupList] = await Promise.all([
        // 只有「外部身份提供方」（category=oauth）才能作为登录方式；邮件/短信等不在此列
        providerApi
          .list({ institution, category: 'oauth', size: 200 })
          .catch(() => ({ items: [] as Provider[] })),
        certApi.list({ institution, size: 200 }).catch(() => ({ items: [] as Cert[] })),
        groupApi.list({ institution, size: 200 }).catch(() => ({ items: [] as Group[] })),
      ])
      if (cancelled) return
      setProviders(providerList.items ?? [])
      setCerts(certList.items ?? [])
      setGroups(groupList.items ?? [])
    })()
    return () => {
      cancelled = true
    }
  }, [institution])

  useEffect(() => {
    if (isCreate || appID <= 0 || !institution) return
    let cancelled = false
    setLoading(true)
    void (async () => {
      try {
        const app = await applicationApi.get(appID, institution)
        if (cancelled) return
        setEditing(app)
        const next = applicationToForm(app)
        setForm(next)
        baseline.current = JSON.stringify(next)
      } catch {
        if (!cancelled) setEditing(null)
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [isCreate, appID, institution])

  // 离开页面前的浏览器级拦截
  useEffect(() => {
    if (!dirty) return
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [dirty])

  const backToList = `/institutions/${institution}/applications`

  const save = useCallback(async () => {
    const check = formToApplicationValidation(form)
    if (!check.ok) {
      toast.error(t(check.reason ?? 'applications.invalidForm'))
      return
    }
    const payload = formToApplication({ ...form, institution_id: Number(institution) })
    if (editing) {
      const ok = await run(() => applicationApi.update(editing.id, payload), 'common.saved')
      if (ok) {
        baseline.current = JSON.stringify(form)
        navigate(backToList)
      }
      return
    }
    let result: ApplicationCreated | null = null
    const ok = await run(async () => {
      result = await applicationApi.create(payload)
    }, 'applications.created')
    if (ok && result) setCreated(result)
  }, [form, institution, editing, run, navigate, backToList, t])

  const requestCancel = useCallback(() => {
    if (dirty) {
      setConfirmDiscard(true)
      return
    }
    navigate(backToList)
  }, [dirty, navigate, backToList])

  const titleKey = isCreate ? 'applications.createTitle' : 'applications.editTitle'

  const preview = useMemo(
    () => (
      <Dialog
        open={previewOpen}
        onOpenChange={(open) => {
          setPreviewOpen(open)
          if (open) setPreviewMode('signin')
        }}
      >
        <DialogContent
          className="sm:max-w-4xl"
          innerClassName="p-0 gap-0"
          // 预览是静态示意：不要让弹窗把焦点移进预览里的输入框（会显示焦点环）
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          <DialogHeader className="border-b px-6 py-4">
            <DialogTitle>{t('applications.previewTitle')}</DialogTitle>
            <DialogDescription>{t('applications.previewHint')}</DialogDescription>
          </DialogHeader>
          {/* 登录页 / 注册页切换：预览同一套品牌与登录方式 */}
          <div className="flex gap-2 px-6 pt-4">
            <Button
              type="button"
              size="sm"
              variant={previewMode === 'signin' ? 'default' : 'outline'}
              onClick={() => setPreviewMode('signin')}
            >
              {t('auth.signIn')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant={previewMode === 'signup' ? 'default' : 'outline'}
              disabled={!form.self_signup_enabled}
              onClick={() => setPreviewMode('signup')}
            >
              {t('auth.signUp')}
            </Button>
          </div>
          <LoginPagePreview
            logo={form.logo}
            displayName={form.name}
            name={form.code}
            description={form.description}
            cardLogo={form.logo}
            accentColor={form.theme_enabled ? form.color_primary : ''}
            methods={signinMethodKinds(form)}
            mode={previewMode}
            allowSignUp={form.self_signup_enabled}
          />
        </DialogContent>
      </Dialog>
    ),
    [previewOpen, previewMode, form, t],
  )

  if (created) {
    const authorizeUrl = buildAuthorizeUrl(created)
    const endpoints = [
      { labelKey: 'applications.discovery', value: `${created.endpoints.issuer}/.well-known/openid-configuration` },
      { labelKey: 'applications.authorize', value: created.endpoints.authorize },
      { labelKey: 'applications.tokenEndpoint', value: created.endpoints.token },
      { labelKey: 'applications.userinfo', value: created.endpoints.userinfo },
      { labelKey: 'applications.jwks', value: created.endpoints.jwks },
    ]
    return (
      <PageShell titleKey={titleKey} subtitleKey="applications.createDoneHint">
        <div className="max-w-3xl space-y-4">
          <Card>
            <CardContent className="space-y-5 pt-6">
              <div className="flex items-center gap-3">
                <span className="bg-success/12 text-success grid size-10 place-items-center rounded-full">
                  <CheckCircle2 className="size-5" />
                </span>
                <div className="space-y-0.5">
                  <p className="text-[15px] font-semibold">{t('applications.createDoneTitle')}</p>
                  <p className="text-muted-foreground text-xs">
                    {t('applications.createDoneBody', { name: created.name || created.code })}
                  </p>
                </div>
              </div>

              <div className="space-y-2">
                <p className="text-muted-foreground text-[11px] font-medium">
                  {t('applications.credentials')}
                </p>
                <div className="flex items-center gap-2 rounded-md border p-2">
                  <span className="text-muted-foreground w-28 shrink-0 text-xs">
                    {t('applications.clientId')}
                  </span>
                  <code className="min-w-0 flex-1 truncate font-mono text-xs">{created.client_id}</code>
                  <CopyButton value={created.client_id} />
                </div>
                <div className="flex items-center gap-2 rounded-md border p-2">
                  <span className="text-muted-foreground w-28 shrink-0 text-xs">
                    {t('applications.clientSecret')}
                  </span>
                  <code className="min-w-0 flex-1 truncate font-mono text-xs">{created.client_secret}</code>
                  <CopyButton value={created.client_secret} />
                </div>
                <p className="text-muted-foreground text-[11px]">{t('applications.secretOnceHint')}</p>
              </div>

              <div className="space-y-2">
                <p className="text-muted-foreground text-[11px] font-medium">
                  {t('applications.authorizeLink')}
                </p>
                <div className="flex items-center gap-2 rounded-md border p-2">
                  <code className="min-w-0 flex-1 truncate font-mono text-xs">{authorizeUrl}</code>
                  <CopyButton value={authorizeUrl} />
                  <Button asChild variant="outline" size="sm">
                    <a href={authorizeUrl} target="_blank" rel="noreferrer">
                      <ExternalLink className="size-4" />
                      {t('applications.openAuthorize')}
                    </a>
                  </Button>
                </div>
                {(created.redirect_uris?.length ?? 0) === 0 ? (
                  <p className="text-warning text-[11px]">{t('applications.noRedirectWarn')}</p>
                ) : (
                  <p className="text-muted-foreground text-[11px]">{t('applications.authorizeLinkHint')}</p>
                )}
              </div>

              <div className="space-y-2">
                <p className="text-muted-foreground text-[11px] font-medium">
                  {t('applications.oauthEndpoints')}
                </p>
                {endpoints.map((row) => (
                  <div key={row.labelKey} className="flex items-center gap-2 rounded-md border p-2">
                    <span className="text-muted-foreground w-28 shrink-0 text-xs">{t(row.labelKey)}</span>
                    <code className="min-w-0 flex-1 truncate font-mono text-xs">{row.value}</code>
                    <CopyButton value={row.value} />
                  </div>
                ))}
              </div>

              {created.self_signup_enabled ? (
                <div className="space-y-2">
                  <p className="text-muted-foreground text-[11px] font-medium">
                    {t('applications.signupLink')}
                  </p>
                  <div className="flex items-center gap-2 rounded-md border p-2">
                    <code className="min-w-0 flex-1 truncate font-mono text-xs">
                      {`${window.location.origin}/signup?client_id=${created.client_id}`}
                    </code>
                    <CopyButton value={`${window.location.origin}/signup?client_id=${created.client_id}`} />
                  </div>
                  <p className="text-muted-foreground text-[11px]">{t('applications.signupLinkHint')}</p>
                </div>
              ) : null}

              <div className="flex flex-wrap gap-2">
                <Button onClick={() => navigate(`/institutions/${institution}/applications/${created.id}`)}>
                  <CheckCircle2 className="size-4" /> {t('applications.openDetail')}
                </Button>
                <Button variant="outline" onClick={() => navigate(backToList)}>
                  <ArrowLeft className="size-4" /> {t('applications.backToList')}
                </Button>
                <Button
                  variant="ghost"
                  onClick={() => {
                    setCreated(null)
                    setForm({ ...emptyApplicationForm, institution_id: Number(institution) })
                    baseline.current = JSON.stringify({ ...emptyApplicationForm, institution_id: Number(institution) })
                  }}
                >
                  <Plus className="size-4" /> {t('applications.createAnother')}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </PageShell>
    )
  }

  return (
    <PageShell
      titleKey={titleKey}
      subtitleKey="applications.subtitle"
      actions={
        <Button variant="outline" onClick={() => setPreviewOpen(true)}>
          <Eye className="size-4" /> {t('applications.previewLogin')}
        </Button>
      }
    >
      {loading ? (
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-28 w-full" />
          ))}
        </div>
      ) : (
        <ApplicationWizard
          form={form}
          onChange={setForm}
          editing={editing}
          institution={institution}
          providers={providers}
          certs={certs}
          groups={groups}
          pending={pending}
          onFinish={() => void save()}
          finishLabelKey={editing ? 'common.save' : 'applications.createSubmit'}
          onCancel={requestCancel}
        />
      )}

      {preview}

      <ConfirmDialog
        open={confirmDiscard}
        onOpenChange={setConfirmDiscard}
        titleKey="applications.unsavedTitle"
        descriptionKey="applications.unsavedBody"
        confirmLabelKey="common.discard"
        onConfirm={() => {
          setConfirmDiscard(false)
          baseline.current = JSON.stringify(form)
          navigate(backToList)
        }}
      />
    </PageShell>
  )
}
