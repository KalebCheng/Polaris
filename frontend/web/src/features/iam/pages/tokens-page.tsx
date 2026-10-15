import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Ban, Trash2 } from 'lucide-react'
import { Badge, Button } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { StatusPill } from '@/shared/components/status-pill'
import { formatDateTime } from '@/shared/lib/format'
import type { Token } from '@/shared/api/types'
import { tokenApi } from '../api'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { ResourceTable, type Column } from '../components/resource-table'
import { SecuritySubnav } from '../components/security-subnav'

/** 令牌管理（平台级，跨组织）：查看已签发令牌并吊销 */
export default function TokensPage() {
  const { t } = useTranslation()
  const list = useListQuery<Token>((params) => tokenApi.list(params), { scope: 'platform' })
  const { pending, run } = useMutation()
  const [revoking, setRevoking] = useState<Token | null>(null)
  const [removing, setRemoving] = useState<Token | null>(null)

  const activeCount = list.items.filter((token) => !token.revoked).length

  const columns: Column<Token>[] = [
    {
      key: 'name',
      headerKey: 'tokens.name',
      render: (token) => <span className="text-foreground font-medium">{token.name}</span>,
    },
    {
      key: 'institution',
      headerKey: 'common.institution',
      render: (token) => <Badge variant="secondary">{token.institution_id}</Badge>,
    },
    { key: 'application', headerKey: 'tokens.application', render: (token) => token.application || '-' },
    { key: 'user', headerKey: 'tokens.user', render: (token) => token.user || '-' },
    {
      key: 'grantType',
      headerKey: 'tokens.grantType',
      render: (token) => <Badge variant="secondary">{token.grant_type || '-'}</Badge>,
    },
    {
      key: 'state',
      headerKey: 'tokens.active',
      render: (token) => (
        <StatusPill tone={token.revoked ? 'danger' : 'success'}>
          {token.revoked ? t('tokens.revoked') : t('tokens.active')}
        </StatusPill>
      ),
    },
    {
      key: 'expire',
      headerKey: 'tokens.expiresIn',
      className: 'text-xs',
      render: (token) => formatDateTime(token.expire_time),
    },
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (token) => (
        <div className="flex items-center justify-end gap-1">
          <Button variant="ghost" size="sm" disabled={token.revoked} onClick={() => setRevoking(token)}>
            <Ban className="size-3.5" /> {t('tokens.revoke')}
          </Button>
          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setRemoving(token)}>
            <Trash2 className="size-3.5" /> {t('common.delete')}
          </Button>
        </div>
      ),
    },
  ]

  return (
    <PageShell titleKey="tokens.title" subtitleKey="tokens.platformSubtitle" subnav={<SecuritySubnav />}>
      <SummaryStrip
        items={[
          { label: t('common.totalResources'), value: list.loading ? '—' : list.total },
          { label: t('tokens.active'), value: list.loading ? '—' : activeCount, tone: 'success' },
          { label: t('common.manageScope'), value: t('common.scopePlatform') },
        ]}
      />
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(token) => token.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        unit={t('common.unitToken')}
        toolbar={
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
            count={t('common.totalCount', { total: list.total, unit: t('common.unitToken') })}
          />
        }
      />

      <ConfirmDialog
        open={Boolean(revoking)}
        onOpenChange={(open) => !open && setRevoking(null)}
        titleKey="tokens.revokeConfirmTitle"
        descriptionKey="tokens.revokeConfirmBody"
        confirmLabelKey="tokens.revoke"
        loading={pending}
        onConfirm={async () => {
          if (!revoking) return
            const ok = await run(() => tokenApi.revoke(revoking.id, String(revoking.institution_id)))
          if (ok) {
            setRevoking(null)
            await list.reload()
          }
        }}
      />

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: removing?.name }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
            const ok = await run(() => tokenApi.remove(removing.id, String(removing.institution_id)), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
