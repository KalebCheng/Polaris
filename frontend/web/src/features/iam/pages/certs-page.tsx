import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
} from '@/shared/ui'
import { SummaryStrip } from '@/widgets/layout/summary-strip'
import { ListToolbar } from '@/widgets/layout/list-toolbar'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { StatusPill } from '@/shared/components/status-pill'
import { formatDateTime } from '@/shared/lib/format'
import type { Cert } from '@/shared/api/types'
import { certApi } from '../api'
import { useCurrentInstitution } from '../institution-context'
import { useListQuery } from '../hooks/use-list-query'
import { useMutation } from '../hooks/use-mutation'
import { Field } from '../components/field'
import { ResourceTable, type Column } from '../components/resource-table'
import { RowActionsMenu } from '@/shared/components/row-actions-menu'
import { usePageHeaderActions } from '../components/page-header-actions'

const emptyForm = {
  institution_id: 0,
  name: '',
  display_name: '',
  bit_size: 2048,
  expire_in_years: 10,
  certificate: '',
  private_key: '',
}

/** 证书管理（平台级，跨组织）：id_token 签名密钥对（留空自动生成自签名证书） */
export default function CertsPage() {
  const { t } = useTranslation()
  const { institutions, institution } = useCurrentInstitution()

  const list = useListQuery<Cert>((params) => certApi.list(params), { scope: 'platform' })
  const { pending, run } = useMutation()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Cert | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [removing, setRemoving] = useState<Cert | null>(null)

  const openCreate = () => {
    setEditing(null)
    setForm({ ...emptyForm, institution_id: Number(institution) || institutions[0]?.id || 0 })
    setDialogOpen(true)
  }

  const openEdit = (cert: Cert) => {
    setEditing(cert)
    setForm({
      institution_id: Number(cert.institution_id),
      name: cert.name,
      display_name: cert.display_name,
      bit_size: cert.bit_size || 2048,
      expire_in_years: cert.expire_in_years || 10,
      certificate: '',
      private_key: '',
    })
    setDialogOpen(true)
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const payload = {
      institution_id: Number(form.institution_id),
      name: form.name,
      display_name: form.display_name,
      scope: 'JWT',
      type: 'x509',
      crypto_algorithm: 'RSA',
      bit_size: Number(form.bit_size) || 2048,
      expire_in_years: Number(form.expire_in_years) || 10,
      certificate: form.certificate,
      private_key: form.private_key,
    }
    const ok = editing
      ? await run(() => certApi.update(editing.id, payload, String(String(editing.institution_id))))
      : await run(() => certApi.create(payload))
    if (ok) {
      setDialogOpen(false)
      await list.reload()
    }
  }

  const columns: Column<Cert>[] = [
    {
      key: 'name',
      headerKey: 'certs.name',
      render: (cert) => (
        <div className="flex items-center gap-2">
          <span className="text-foreground font-medium">{cert.name}</span>
          {cert.is_system ? <Badge variant="outline">{t('certs.isSystem')}</Badge> : null}
        </div>
      ),
    },
    {
      key: 'institution',
      headerKey: 'common.institution',
      render: (cert) => <Badge variant="secondary">{cert.institution_id}</Badge>,
    },
    {
      key: 'scope',
      headerKey: 'certs.scope',
      render: (cert) => <Badge variant="secondary">{cert.scope}</Badge>,
    },
    {
      key: 'algorithm',
      headerKey: 'certs.algorithm',
      className: 'text-xs',
      render: (cert) => `${cert.crypto_algorithm} ${cert.bit_size}`,
    },
    {
      key: 'expire',
      headerKey: 'certs.expireTime',
      className: 'text-xs',
      render: (cert) => formatDateTime(cert.expire_time),
    },
    {
      key: 'privateKey',
      headerKey: 'certs.hasPrivateKey',
      render: (cert) => (
        <StatusPill tone={cert.has_private_key ? 'success' : 'warning'}>
          {cert.has_private_key ? t('common.yes') : t('common.no')}
        </StatusPill>
      ),
    },
    {
      key: 'actions',
      headerKey: 'common.actions',
      headClassName: 'text-right',
      render: (cert) => (
        <div className="flex justify-end">
          <RowActionsMenu
            primary={{ labelKey: 'common.edit', onSelect: () => openEdit(cert) }}
            actions={
              cert.is_system
                ? []
                : [{ labelKey: 'common.delete', destructive: true, onSelect: () => setRemoving(cert) }]
            }
          />
        </div>
      ),
    },
  ]

  // 页头操作：按钮与页面内的新建弹窗耦合，所以仍由页面渲染，只是 portal 到组级布局的页头里
  const headerActions = usePageHeaderActions(
    <Button onClick={openCreate}>
      <Plus className="size-4" /> {t('common.create')}
    </Button>,
  )

  return (
    <>
      {headerActions}
      <SummaryStrip
        items={[
          { label: t('common.totalResources'), value: list.loading ? '—' : list.total },
          { label: t('common.manageScope'), value: t('common.scopePlatform') },
        ]}
      />
      <ResourceTable
        columns={columns}
        rows={list.items}
        rowKey={(cert) => cert.id}
        loading={list.loading}
        page={list.page}
        size={list.size}
        total={list.total}
        onPageChange={list.setPage}
        unit={t('common.unitCert')}
        toolbar={
          <ListToolbar
            keyword={list.keyword}
            onKeywordChange={list.setKeyword}
            onSubmit={list.search}
            count={t('common.totalCount', { total: list.total, unit: t('common.unitCert') })}
          />
        }
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t('certs.name') : t('certs.createTitle')}</DialogTitle>
            <DialogDescription>{t('certs.createHint')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <Field id="cert-institution" labelKey="common.institution">
              <Select
                value={String(form.institution_id || '')}
                onValueChange={(value) => setForm({ ...form, institution_id: Number(value) })}
                disabled={Boolean(editing)}
              >
                <SelectTrigger id="cert-institution">
                  <SelectValue placeholder={t('auth.institutionPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {institutions.map((option) => (
                    <SelectItem key={option.name} value={option.name}>
                      {option.display_name || option.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field id="cert-name" labelKey="certs.name">
              <Input
                id="cert-name"
                value={form.name}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
                disabled={Boolean(editing)}
                required
              />
            </Field>
            <Field id="cert-display" labelKey="certs.displayName">
              <Input
                id="cert-display"
                value={form.display_name}
                onChange={(event) => setForm({ ...form, display_name: event.target.value })}
              />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="cert-bits" labelKey="certs.bitSize">
                <Select
                  value={String(form.bit_size)}
                  onValueChange={(value) => setForm({ ...form, bit_size: Number(value) })}
                >
                  <SelectTrigger id="cert-bits">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="2048">2048</SelectItem>
                    <SelectItem value="3072">3072</SelectItem>
                    <SelectItem value="4096">4096</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field id="cert-years" labelKey="certs.expireInYears">
                <Input
                  id="cert-years"
                  type="number"
                  min={1}
                  value={form.expire_in_years}
                  onChange={(event) => setForm({ ...form, expire_in_years: Number(event.target.value) })}
                />
              </Field>
            </div>
            <Field id="cert-pem" labelKey="certs.certificate">
              <Textarea
                id="cert-pem"
                value={form.certificate}
                onChange={(event) => setForm({ ...form, certificate: event.target.value })}
                rows={3}
                placeholder="-----BEGIN CERTIFICATE-----"
              />
            </Field>
            <Field id="cert-key" labelKey="certs.hasPrivateKey">
              <Textarea
                id="cert-key"
                value={form.private_key}
                onChange={(event) => setForm({ ...form, private_key: event.target.value })}
                rows={3}
                placeholder="-----BEGIN RSA PRIVATE KEY-----"
              />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={pending || !form.institution_id}>
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(null)}
        titleKey="common.deleteConfirmTitle"
        descriptionKey="common.deleteConfirmBody"
        descriptionParams={{ name: removing?.name }}
        loading={pending}
        onConfirm={async () => {
          if (!removing) return
          const ok = await run(() => certApi.remove(removing.id, String(removing.institution_id)), 'common.deleted')
          if (ok) {
            setRemoving(null)
            await list.reload()
          }
        }}
      />
    </>
  )
}
