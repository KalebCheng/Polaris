import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Button } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import type { Institution } from '@/shared/api/types'
import { institutionApi } from '../api'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { ResourceCards } from '../components/resource-cards'
import { InstitutionCard } from '../components/institution-card'

/**
 * 组织管理：多租户命名空间的列表、进入与删除。
 * 列表用**卡片网格**而非表格——组织的标识是图标 + 名称，摊成表格反而看不清"这是个什么组织"。
 * 新建 / 编辑走独立页面（`/institutions/new`、`/institutions/:name/edit`），
 * 不再把长表单塞进弹窗（见 docs/design-language.md §4）。
 */
export default function InstitutionsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const list = useListQuery<Institution>(institutionApi.list)
  const { pending, run } = useMutation()
  const [removing, setRemoving] = useState<Institution | null>(null)

  return (
    <PageShell
      titleKey="institutions.title"
      subtitleKey="institutions.subtitle"
      actions={
        <Button onClick={() => navigate('/institutions/new')}>
          <Plus className="size-4" /> {t('common.create')}
        </Button>
      }
    >
      <SummaryStrip
        items={[
          { label: t('institutions.totalOrgs'), value: list.loading ? '—' : list.total },
          { label: t('common.manageScope'), value: t('common.scopePlatform') },
        ]}
      />
      <ResourceCards
        items={list.items}
        cardKey={(org) => org.name}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        unit={t('common.unitOrg')}
        renderCard={(org) => (
          <InstitutionCard
            org={org}
            onEdit={() => navigate(`/institutions/${org.id}/edit`)}
            onDelete={() => setRemoving(org)}
          />
        )}
        emptyAction={
          <Button variant="outline" onClick={() => navigate('/institutions/new')}>
            {t('common.create')}
          </Button>
        }
        toolbar={
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
            placeholder={t('institutions.searchPlaceholder')}
            count={t('common.totalCount', { total: list.total, unit: t('common.unitOrg') })}
          />
        }
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
          const ok = await run(() => institutionApi.remove(removing.id), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}
