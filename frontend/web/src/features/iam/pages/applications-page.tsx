import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { ExternalLink, Plus } from 'lucide-react'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Label,
} from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { CopyButton } from '@/shared/components/copy-button'
import { formatDateTime } from '@/shared/lib/format'
import type { Application } from '@/shared/api/types'
import { applicationApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { buildAuthorizeUrl } from '../lib/authorize-url'
import { ResourceCards } from '../components/resource-cards'
import { ApplicationCard } from '../components/application-card'

/** 应用管理：卡片网格列表 + 跳转单页编辑（登录策略 / 提供方 / 令牌 / 品牌） */
export default function ApplicationsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const institution = useWorkspaceInstitution()
  const list = useListQuery<Application>((params) => applicationApi.list(params), {
    institution: institution || undefined,
  })
  const { pending, run } = useMutation()

  const [removing, setRemoving] = useState<Application | null>(null)
  const [resetting, setResetting] = useState<Application | null>(null)
  const [selected, setSelected] = useState<Application | null>(null)
  /** 重置密钥后一次性展示的新密钥（刷新/关闭即消失） */
  const [revealedSecret, setRevealedSecret] = useState('')

  const endpointRows = selected
    ? [
        {
          label: t('applications.discovery'),
          value: `${window.location.origin}/.well-known/openid-configuration`,
        },
        { label: t('applications.jwks'), value: `${window.location.origin}/.well-known/jwks` },
        { label: t('applications.authorize'), value: `${window.location.origin}/oauth/authorize` },
        { label: t('applications.tokenEndpoint'), value: `${window.location.origin}/oauth/token` },
        { label: t('applications.userinfo'), value: `${window.location.origin}/oauth/userinfo` },
      ]
    : []

  const selectedAuthorizeUrl = selected ? buildAuthorizeUrl(selected) : ''

  return (
    <PageShell
      titleKey="applications.title"
      subtitleKey="applications.subtitle"
      actions={
        <Button
          onClick={() => navigate(`/institutions/${institution}/applications/new`)}
          disabled={!institution}
        >
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <ResourceCards
        items={list.items}
        cardKey={(app) => app.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        renderCard={(app) => (
          <ApplicationCard
            app={app}
            editHref={`/institutions/${institution}/applications/${app.id}`}
            onEndpoints={() => setSelected(app)}
            onResetSecret={() => setResetting(app)}
            onDelete={() => setRemoving(app)}
          />
        )}
        toolbar={
          <ListToolbar keyword={list.keyword} onKeywordChange={list.setKeyword} onSubmit={list.search} />
        }
      />

      <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t('applications.oauthEndpoints')}</DialogTitle>
            <DialogDescription>
              {selected?.name || selected?.code} · {t('applications.clientId')}: {selected?.client_id}
            </DialogDescription>
          </DialogHeader>
          {selected ? (
            <div className="space-y-2">
              <Label className="text-xs">{t('applications.authorizeLink')}</Label>
              <div className="flex items-center gap-2 rounded-md border p-2">
                <code className="min-w-0 flex-1 truncate font-mono text-xs">{selectedAuthorizeUrl}</code>
                <CopyButton value={selectedAuthorizeUrl} />
                <Button asChild variant="outline" size="sm">
                  <a href={selectedAuthorizeUrl} target="_blank" rel="noreferrer">
                    <ExternalLink className="size-4" />
                    {t('applications.openAuthorize')}
                  </a>
                </Button>
              </div>
              {(selected.redirect_uris?.length ?? 0) === 0 ? (
                <p className="text-warning text-[11px]">{t('applications.noRedirectWarn')}</p>
              ) : (
                <p className="text-muted-foreground text-[11px]">{t('applications.authorizeLinkHint')}</p>
              )}
            </div>
          ) : null}
          <div className="space-y-2">
            {endpointRows.map((row) => (
              <div key={row.label} className="flex items-center gap-2 rounded-md border p-2">
                <Label className="w-32 shrink-0 text-xs">{row.label}</Label>
                <code className="min-w-0 flex-1 truncate font-mono text-xs">{row.value}</code>
                <CopyButton value={row.value} />
              </div>
            ))}
          </div>
          <Label className="text-xs">{t('applications.clientSecret')}</Label>
          {revealedSecret ? (
            <>
              <div className="flex items-center gap-2">
                <code className="min-w-0 flex-1 truncate rounded-md border p-2 font-mono text-xs">
                  {revealedSecret}
                </code>
                <CopyButton value={revealedSecret} />
              </div>
              <p className="text-muted-foreground text-xs">{t('applications.secretOnceHint')}</p>
            </>
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={selected?.has_client_secret ? 'secondary' : 'outline'}>
                {selected?.has_client_secret ? t('applications.secretConfigured') : t('applications.secretMissing')}
              </Badge>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setResetting(selected ?? null)}
                disabled={!selected}
              >
                {t('applications.resetSecret')}
              </Button>
            </div>
          )}
          <p className="text-muted-foreground text-xs">
            {t('common.createdAt')}: {formatDateTime(selected?.created_at)}
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSelected(null)}>
              {t('common.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: removing?.name || removing?.code }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(() => applicationApi.remove(removing.id), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />

      <ConfirmDialog
        open={Boolean(resetting)}
        onOpenChange={(open) => !open && setResetting(null)}
        titleKey="applications.resetSecretConfirmTitle"
        descriptionKey="applications.resetSecretConfirmBody"
        confirmLabelKey="applications.resetSecret"
        loading={pending}
        onConfirm={async () => {
          if (!resetting) return
          const ok = await run(async () => {
            const result = await applicationApi.resetSecret(resetting.id)
            setRevealedSecret(result.client_secret)
          })
          if (ok) {
            setResetting(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
