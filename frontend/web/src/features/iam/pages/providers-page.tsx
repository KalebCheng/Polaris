import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Button, Skeleton, Tabs, TabsList, TabsTrigger } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { ProviderIcon, ProviderTile } from '@/shared/components/provider-icon'
import { ConnectionCard, type ConnectionCardView } from '../components/connection-card'
import type { Provider, ProviderCatalog } from '@/shared/api/types'
import { providerApi } from '../api'
import { useWorkspaceInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'

/**
 * 提供商管理：外部身份提供方（第三方登录 / 标准 OIDC）、邮件、短信、人机校验、对象存储。
 *
 * 形态对齐「身份连接」：类别标签页 + 实例卡片（状态 / 检查连通性 / 修改 / 删除）；
 * 新增与编辑一律走**整页向导**（/providers/new 与 /providers/:id），不使用弹窗。
 */
export default function ProvidersPage() {
  const { t } = useTranslation()
  const institution = useWorkspaceInstitution()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const category = params.get('category') ?? 'oauth'

  const [catalog, setCatalog] = useState<ProviderCatalog | null>(null)
  const [removing, setRemoving] = useState<Provider | null>(null)
  const [testingID, setTestingID] = useState(0)
  const { pending, run } = useMutation()

  const list = useListQuery<Provider>((query) => providerApi.list(query), {
    institution: institution || undefined,
  })

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const data = await providerApi.catalog()
        if (!cancelled) setCatalog(data)
      } catch {
        if (!cancelled) setCatalog(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const openNew = () => navigate(`/institutions/${institution}/providers/new?category=${category}`)
  const openEdit = (provider: Provider) => navigate(`/institutions/${institution}/providers/${provider.id}`)

  const items = list.items.filter((item) => item.category === category)
  // 单例类别（邮件/短信/人机校验/存储）运行时只取"启用中的那一个"，故在卡片上显式标出"当前生效"，
  // 避免再出现"配了两个存储，上传到底用哪个"的疑问
  const singleton = Boolean(catalog?.categories.find((group) => group.key === category)?.singleton)
  const switchState = async (provider: Provider) => {
    const next = provider.state === 'active' ? 'disabled' : 'active'
    const ok = await run(() => providerApi.setState(provider.id, next), 'common.saved')
    if (ok) await list.reload()
  }

  const cards: ConnectionCardView[] = items.map((provider) => {
    const typeInfo = catalog?.categories
      .find((group) => group.key === provider.category)
      ?.types.find((type) => type.key === provider.type)
    const iconKey = typeInfo?.icon ?? provider.category
    const endpoint =
      str(provider.config['issuer_url']) ||
      str(provider.config['host']) ||
      str(provider.config['endpoint']) ||
      str(provider.config['custom_auth_url'])
    return {
      name: provider.display_name || provider.name,
      subtitle: provider.name,
      icon: { key: iconKey },
      type: provider.type,
      tags: [
        t(`providers.category.${provider.category}`),
        provider.type,
        ...(singleton && provider.state === 'active' ? [t('providers.inUse')] : []),
      ],
      meta: [
        endpoint || t('providers.noEndpoint'),
        provider.has_secret ? t('providers.secretConfigured') : t('providers.secretMissing'),
      ],
      status: {
        tone: provider.state === 'active' ? 'success' : 'neutral',
        label: provider.state === 'active' ? t('common.enabled') : t('common.disabled'),
      },
      sections: [
        {
          label: t('providers.connectionSection'),
          hint: endpoint || t('providers.noEndpoint'),
          links: [
            {
              label: testingID === provider.id && pending ? t('providers.testing') : t('providers.test'),
              onClick: () => void checkNow(provider),
            },
            ...(singleton
              ? [
                  {
                    label: provider.state === 'active' ? t('common.disable') : t('common.enable'),
                    onClick: () => void switchState(provider),
                  },
                ]
              : []),
            { label: t('common.edit'), onClick: () => openEdit(provider) },
          ],
        },
      ],
      onEdit: () => openEdit(provider),
      onDelete: () => setRemoving(provider),
    }
  })

  async function checkNow(provider: Provider) {
    setTestingID(provider.id)
    await run(() => providerApi.test(provider.id), 'providers.testOk')
    setTestingID(0)
  }

  return (
    <PageShell
      titleKey="providers.title"
      subtitleKey="providers.subtitle"
      actions={
        <Button onClick={openNew} disabled={!institution}>
          <Plus className="size-4" /> {t('providers.add')}
        </Button>
      }
    >
      <div className="space-y-5">
        <Tabs value={category} onValueChange={(value) => setParams({ category: value })}>
          <TabsList className="flex-wrap">
            {(catalog?.categories ?? []).map((group) => (
              <TabsTrigger key={group.key} value={group.key}>
                <span className="inline-flex items-center gap-1.5">
                  {/* 图标 key 由后端目录声明（CategoryInfo.Icon），前端不另起一份映射 */}
                  <ProviderIcon iconKey={group.icon || group.key} size={16} />
                  {t(`providers.category.${group.key}`)}
                </span>
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <p className="text-muted-foreground text-xs">{t(`providers.categoryHint.${category}`)}</p>

        <div className="space-y-3">
          {list.loading ? (
            <>
              <Skeleton className="h-32 w-full" />
              <Skeleton className="h-32 w-full" />
            </>
          ) : cards.length ? (
            cards.map((view) => <ConnectionCard key={view.subtitle} view={view} />)
          ) : (
            <div className="surface rounded-2xl p-8 text-center">
              <ProviderTile iconKey={category} tileSize={48} />
              <p className="mt-3 text-sm font-medium">{t('providers.empty')}</p>
              <p className="text-muted-foreground mt-1 text-xs">{t('providers.emptyHint')}</p>
              <Button className="mt-4" variant="outline" onClick={openNew}>
                <Plus className="size-4" /> {t('providers.add')}
              </Button>
            </div>
          )}
        </div>
      </div>

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: removing?.display_name || removing?.name }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(() => providerApi.remove(removing.id), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </PageShell>
  )
}

function str(value: unknown): string {
  return typeof value === 'string' ? value : ''
}
