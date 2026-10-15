import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { KeySquare, Link2, PlugZap, UserCog } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import { LoadError } from '@/shared/components/load-error'
import { CopyButton } from '@/shared/components/copy-button'
import { ListPagination } from '@/widgets/layout/list-pagination'
import { formatDateTime } from '@/shared/lib/format'
import { describeError } from '@/shared/lib/errors'
import type { AppAccount, ProvisionHistory, User, UserIdentity } from '@/shared/api/types'
import { userApi } from '../../api'
import { useWorkspaceInstitutionId } from '../../institution-context'
import { useMutation } from '../../hooks/use-mutation'
import { useOnceKey } from '../../hooks/use-once-key'
import { Field } from '../field'

interface RelationsTabProps {
  user: User
  onChanged: () => Promise<void> | void
}

const PAGE_SIZE = 10

/**
 * 关联标签：外部身份绑定 / 下游应用账号 / 供给历史 / API 令牌。
 *
 * 这三块回答"这个人跟平台之外的系统是什么关系"：怎么进来的（身份绑定）、
 * 被发到了哪些下游（应用账号）、发的过程是否正常（供给历史）。
 */
export function RelationsTab({ user, onChanged }: RelationsTabProps) {
  const { t } = useTranslation()
  const institutionId = useWorkspaceInstitutionId()
  const { pending, run } = useMutation()

  const [identities, setIdentities] = useState<UserIdentity[]>([])
  const [accounts, setAccounts] = useState<AppAccount[]>([])
  const [history, setHistory] = useState<ProvisionHistory[]>([])
  const [historyTotal, setHistoryTotal] = useState(0)
  const [historyPage, setHistoryPage] = useState(1)
  const [loading, setLoading] = useState(true)
  // 三块数据各自独立：读不到时必须显式报错，不能渲染成"没有绑定 / 没有下游账号"
  const [errors, setErrors] = useState({ identities: '', accounts: '', history: '' })
  const [unbinding, setUnbinding] = useState<UserIdentity | null>(null)

  const [apiKeyOpen, setApiKeyOpen] = useState(false)
  const [apiKeyName, setApiKeyName] = useState('')
  const [apiKeyHours, setApiKeyHours] = useState(720)
  const [issuedToken, setIssuedToken] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setErrors({ identities: '', accounts: '', history: '' })
    const [identityResult, accountResult, historyResult] = await Promise.allSettled([
      userApi.identities(user.id),
      userApi.appAccounts(user.id),
      userApi.provisionHistory(user.id, { page: historyPage, size: PAGE_SIZE }),
    ])
    if (identityResult.status === 'fulfilled') {
      setIdentities(identityResult.value.items ?? [])
    } else {
      setErrors((prev) => ({ ...prev, identities: describeError(identityResult.reason, t) }))
    }
    if (accountResult.status === 'fulfilled') {
      setAccounts(accountResult.value.items ?? [])
    } else {
      setErrors((prev) => ({ ...prev, accounts: describeError(accountResult.reason, t) }))
    }
    if (historyResult.status === 'fulfilled') {
      setHistory(historyResult.value.items ?? [])
      setHistoryTotal(historyResult.value.total ?? 0)
    } else {
      setErrors((prev) => ({ ...prev, history: describeError(historyResult.reason, t) }))
    }
    setLoading(false)
  }, [user.id, historyPage, t])

  useOnceKey(`relations:${user.id}:${historyPage}`, load)

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Link2 className="size-4" /> {t('users.identityTitle')}
          </CardTitle>
          <CardDescription>{t('users.identityHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : errors.identities ? (
            <LoadError compact onRetry={() => void load()} detail={errors.identities} />
          ) : identities.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('users.identityEmpty')}</p>
          ) : (
            <ul className="divide-border/60 divide-y rounded-xl border">
              {identities.map((identity) => (
                <li key={identity.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                  <div className="min-w-0">
                    <p className="font-medium">{identity.provider}</p>
                    <p className="text-muted-foreground truncate text-xs">
                      {identity.provider_user_id}
                      {identity.email ? ` · ${identity.email}` : ''}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive"
                    onClick={() => setUnbinding(identity)}
                  >
                    {t('users.unbind')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <UserCog className="size-4" /> {t('users.appAccountsTitle')}
          </CardTitle>
          <CardDescription>{t('users.appAccountsHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : errors.accounts ? (
            <LoadError compact onRetry={() => void load()} detail={errors.accounts} />
          ) : accounts.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('users.appAccountsEmpty')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('appAccounts.application')}</TableHead>
                  <TableHead>{t('appAccounts.relatedUsername')}</TableHead>
                  <TableHead>{t('appAccounts.status')}</TableHead>
                  <TableHead>{t('appAccounts.createType')}</TableHead>
                  <TableHead>{t('appAccounts.createdAt')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {accounts.map((account) => (
                  <TableRow key={account.id}>
                    <TableCell>{account.application}</TableCell>
                    <TableCell className="font-mono text-xs">{account.related_username || account.username}</TableCell>
                    <TableCell>
                      <Badge variant={account.status === 'active' ? 'success' : 'destructive'}>{account.status}</Badge>
                    </TableCell>
                    <TableCell className="text-xs">{account.create_type}</TableCell>
                    <TableCell className="text-xs">{formatDateTime(account.created_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          <div className="mt-3">
            <Link
              to={`/institutions/${institutionId}/app-accounts`}
              className="text-primary inline-flex items-center gap-1 text-sm hover:underline"
            >
              <PlugZap className="size-3.5" /> {t('users.appAccountsManage')}
            </Link>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('users.provisionHistoryTitle')}</CardTitle>
          <CardDescription>{t('users.provisionHistoryHint')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <Skeleton className="h-24 w-full" />
          ) : errors.history ? (
            <LoadError onRetry={() => void load()} detail={errors.history} />
          ) : history.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('users.provisionHistoryEmpty')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('provisioning.connector')}</TableHead>
                  <TableHead>{t('provisioning.application')}</TableHead>
                  <TableHead>{t('provisioning.action')}</TableHead>
                  <TableHead>{t('provisioning.status')}</TableHead>
                  <TableHead>{t('provisioning.message')}</TableHead>
                  <TableHead>{t('provisioning.createdAt')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className="text-xs">{item.connector || '-'}</TableCell>
                    <TableCell className="text-xs">{item.application || '-'}</TableCell>
                    <TableCell className="text-xs">{item.action}</TableCell>
                    <TableCell>
                      <Badge variant={item.status === 'success' ? 'success' : item.status === 'failed' ? 'destructive' : 'outline'}>
                        {item.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[16rem] truncate text-xs">{item.message || '-'}</TableCell>
                    <TableCell className="text-xs">{formatDateTime(item.created_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          <ListPagination page={historyPage} size={PAGE_SIZE} total={historyTotal} onPageChange={setHistoryPage} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <KeySquare className="size-4" /> {t('users.apiKeyTitle')}
          </CardTitle>
          <CardDescription>{t('users.apiKeyHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" size="sm" onClick={() => setApiKeyOpen(true)}>
            <KeySquare className="size-4" /> {t('users.issueApiKey')}
          </Button>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={Boolean(unbinding)}
        onOpenChange={(open) => !open && setUnbinding(null)}
        titleKey="users.unbindConfirmTitle"
        descriptionKey="users.unbindConfirmBody"
        loading={pending}
        onConfirm={async () => {
          if (!unbinding) return
          const ok = await run(async () => {
            await userApi.unbindIdentity(user.id, unbinding.id)
            await load()
            await onChanged()
          })
          if (ok) setUnbinding(null)
        }}
      />

      <Dialog
        open={apiKeyOpen}
        onOpenChange={(open) => {
          setApiKeyOpen(open)
          if (!open) {
            setIssuedToken(null)
            setApiKeyName('')
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('users.apiKeyTitle')}</DialogTitle>
            <DialogDescription>{user.name}</DialogDescription>
          </DialogHeader>
          {issuedToken ? (
            <div className="space-y-3">
              <p className="text-sm">{t('users.apiKeyIssued')}</p>
              <div className="bg-muted/40 border-border/70 flex items-center gap-2 rounded-lg border p-2">
                <code className="min-w-0 flex-1 truncate font-mono text-xs">{issuedToken}</code>
                <CopyButton value={issuedToken} />
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              <Field id="user-api-key-name" labelKey="users.apiKeyName">
                <Input
                  id="user-api-key-name"
                  value={apiKeyName}
                  onChange={(event) => setApiKeyName(event.target.value)}
                  placeholder={`api-${user.name}`}
                />
              </Field>
              <Field id="user-api-key-hours" labelKey="users.apiKeyExpire">
                <Input
                  id="user-api-key-hours"
                  type="number"
                  min={1}
                  value={apiKeyHours}
                  onChange={(event) => setApiKeyHours(Number(event.target.value))}
                />
              </Field>
            </div>
          )}
          <DialogFooter>
            {issuedToken ? (
              <Button onClick={() => setApiKeyOpen(false)}>{t('common.close')}</Button>
            ) : (
              <>
                <Button variant="outline" onClick={() => setApiKeyOpen(false)}>
                  {t('common.cancel')}
                </Button>
                <Button
                  disabled={pending}
                  onClick={async () => {
                    let token: string | null = null
                    const ok = await run(async () => {
                      const result = await userApi.issueApiKey(user.id, {
                        name: apiKeyName || undefined,
                        expire_in_hours: apiKeyHours,
                      })
                      token = result.token
                    })
                    if (ok) setIssuedToken(token)
                  }}
                >
                  {t('common.confirm')}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
