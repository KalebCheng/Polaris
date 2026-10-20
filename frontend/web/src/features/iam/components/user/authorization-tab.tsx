import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Save, ShieldHalf } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  MultiSelect,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/shared/ui'
import type { User, UserAuthorizations } from '@/shared/api/types'
import { LoadError } from '@/shared/components/load-error'
import { describeError } from '@/shared/lib/errors'
import { jobTitleApi, roleApi, userApi } from '../../api'
import { useMutation } from '../../hooks/use-mutation'
import { useOnceKey } from '../../hooks/use-once-key'

interface AuthorizationTabProps {
  user: User
  onChanged: () => Promise<void> | void
}

/**
 * 授权标签：能回答"这个人的权限到底从哪来"。
 *
 * 有效权限由三处并集而成（直接绑定 / 经角色（含嵌套）/ 经职务），
 * 过去只能看到结果看不到来源，出问题时无法判断"该收回哪一条"。
 */
export function AuthorizationTab({ user, onChanged }: AuthorizationTabProps) {
  const { t } = useTranslation()
  const { pending, run } = useMutation()
  const [data, setData] = useState<UserAuthorizations | null>(null)
  const [loading, setLoading] = useState(true)
  const [roleOptions, setRoleOptions] = useState<{ id: number; name: string; displayName: string }[]>([])
  const [groupOptions, setGroupOptions] = useState<{ id: number; name: string; displayName: string }[]>([])
  const [roleIDs, setRoleIDs] = useState<number[]>([])
  const [jobTitleIDs, setJobTitleIDs] = useState<number[]>([])
  // 读不到授权数据时必须显式报错：否则会显示成"这个人没有任何权限"
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    const [authzResult, roleResult, groupResult] = await Promise.allSettled([
      userApi.authorizations(user.id),
      roleApi.list({ page: 1, size: 200 }),
      jobTitleApi.list({ page: 1, size: 200 }),
    ])
    if (authzResult.status === 'fulfilled') {
      setData(authzResult.value)
      setRoleIDs(authzResult.value.direct_role_ids)
      setJobTitleIDs(authzResult.value.job_title_ids)
    } else {
      setError(describeError(authzResult.reason, t))
    }
    // 候选列表只用于选择器，失败时留空即可（不会造成"没有权限"的误判）
    setRoleOptions(
      roleResult.status === 'fulfilled'
        ? roleResult.value.items.map((role) => ({
            id: role.id,
            name: role.name,
            displayName: role.display_name || role.name,
          }))
        : [],
    )
    setGroupOptions(
      groupResult.status === 'fulfilled'
        ? groupResult.value.items.map((group) => ({
            id: group.id,
            name: group.name,
            displayName: group.display_name || group.name,
          }))
        : [],
    )
    setLoading(false)
  }, [user.id, t])

  useOnceKey(`authorizations:${user.id}`, load)

  const rolesDirty =
    JSON.stringify([...roleIDs].toSorted()) !== JSON.stringify([...(data?.direct_role_ids ?? [])].toSorted())
  const jobTitlesDirty =
    JSON.stringify([...jobTitleIDs].toSorted()) !==
    JSON.stringify([...(data?.job_title_ids ?? [])].toSorted())

  return (
    <div className="space-y-4">
      {error ? <LoadError onRetry={() => void load()} detail={error} /> : null}
      <div className="grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('users.directRoles')}</CardTitle>
            <CardDescription>{t('users.directRolesHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <MultiSelect
              options={roleOptions.map((role) => ({ value: String(role.id), label: role.displayName }))}
              value={roleIDs.map(String)}
              onChange={(values) => setRoleIDs(values.map(Number))}
              placeholder={t('users.rolesPlaceholder')}
            />
            <Button
              size="sm"
              variant="outline"
              disabled={pending || !rolesDirty}
              onClick={async () => {
                const ok = await run(async () => {
                  await userApi.setRoles(user.id, roleIDs)
                  await load()
                  await onChanged()
                })
                if (!ok) await load()
              }}
            >
              <Save className="size-4" /> {t('common.save')}
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('users.job_titlesLabel')}</CardTitle>
            <CardDescription>{t('users.job_titlesHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <MultiSelect
              options={groupOptions.map((group) => ({ value: String(group.id), label: group.displayName }))}
              value={jobTitleIDs.map(String)}
              onChange={(values) => setJobTitleIDs(values.map(Number))}
              placeholder={t('users.job_titlesPlaceholder')}
            />
            <Button
              size="sm"
              variant="outline"
              disabled={pending || !jobTitlesDirty}
              onClick={async () => {
                const ok = await run(async () => {
                  await userApi.setJobTitles(user.id, jobTitleIDs)
                  await load()
                  await onChanged()
                })
                if (!ok) await load()
              }}
            >
              <Save className="size-4" /> {t('common.save')}
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('users.inheritedRoles')}</CardTitle>
            <CardDescription>{t('users.inheritedRolesHint')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {loading ? (
              <Skeleton className="h-10 w-full" />
            ) : (
              <>
                <div className="space-y-2">
                  <p className="text-muted-foreground text-xs">{t('users.inheritedRolesLabel')}</p>
                  <div className="flex flex-wrap gap-1.5">
                    {(data?.inherited_roles ?? []).length === 0 ? (
                      <span className="text-muted-foreground text-sm">-</span>
                    ) : (
                      (data?.inherited_roles ?? []).map((role) => (
                        <Badge key={role} variant="secondary">
                          {role}
                        </Badge>
                      ))
                    )}
                  </div>
                </div>
                <div className="space-y-2">
                  <p className="text-muted-foreground text-xs">{t('users.directPermissions')}</p>
                  <div className="flex flex-wrap gap-1.5">
                    {(data?.direct_permissions ?? []).length === 0 ? (
                      <span className="text-muted-foreground text-sm">-</span>
                    ) : (
                      (data?.direct_permissions ?? []).map((permission) => (
                        <Badge key={permission} variant="outline">
                          {permission}
                        </Badge>
                      ))
                    )}
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldHalf className="size-4" /> {t('users.effectivePermissions')}
          </CardTitle>
          <CardDescription>{t('users.effectivePermissionsHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <Skeleton className="h-32 w-full" />
          ) : error ? (
            <LoadError onRetry={() => void load()} detail={error} />
          ) : (data?.effective_permissions ?? []).length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('users.effectivePermissionsEmpty')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('permissions.resources')}</TableHead>
                  <TableHead>{t('permissions.actions')}</TableHead>
                  <TableHead>{t('permissions.effect')}</TableHead>
                  <TableHead>{t('users.permissionSource')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(data?.effective_permissions ?? []).map((rule, index) => (
                  <TableRow key={`${rule.name}-${rule.resource}-${rule.action}-${index}`}>
                    <TableCell className="font-mono text-xs">{rule.resource}</TableCell>
                    <TableCell className="font-mono text-xs">{rule.action}</TableCell>
                    <TableCell>
                      <Badge variant={rule.effect === 'Deny' ? 'destructive' : 'success'}>
                        {rule.effect}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {rule.sources.length === 0 ? (
                          <span className="text-muted-foreground text-xs">-</span>
                        ) : (
                          rule.sources.map((source) => (
                            <Badge key={source} variant="outline" className="font-mono text-[10px]">
                              {source}
                            </Badge>
                          ))
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
