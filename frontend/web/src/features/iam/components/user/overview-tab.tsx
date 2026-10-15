import { useTranslation } from 'react-i18next'
import { Badge, Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui'
import { formatDateTime, formatRelative } from '@/shared/lib/format'
import type { User } from '@/shared/api/types'

interface OverviewTabProps {
  user: User
}

/** 只读信息行：标签在左、值在右；空值统一以 `-` 占位，避免"这行到底有没有值"的猜测 */
function InfoRow({ labelKey, value }: { labelKey: string; value?: React.ReactNode }) {
  const { t } = useTranslation()
  const empty = value === undefined || value === null || value === '' || value === false
  return (
    <div className="flex items-start justify-between gap-4 text-sm">
      <dt className="text-muted-foreground shrink-0">{t(labelKey)}</dt>
      <dd className="min-w-0 truncate text-right">{empty ? '-' : value}</dd>
    </div>
  )
}

/**
 * 概览标签：**只读**的用户事实视图。
 *
 * 边界（第五阶段验收期修正）：详情只负责"看清这个人现在的样子"——身份标识、资料全量、
 * 归属与活跃度。任何写入（改资料、改部门、禁用、重置凭证）都不在这里发生：
 * 资料类改动走 `users/:id/edit`，安全类动作在「安全与认证」标签或头部动作菜单。
 */
export function OverviewTab({ user }: OverviewTabProps) {
  const { t } = useTranslation()
  const verifiedCount = [user.email && user.email_verified, user.phone && user.phone_verified].filter(Boolean).length

  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle className="text-base">{t('users.sectionProfile')}</CardTitle>
          <CardDescription>{t('users.profileReadonlyHint')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-3">
            <h3 className="text-[13px] font-semibold tracking-tight">{t('users.sectionProfileBasic')}</h3>
            <dl className="grid gap-3 sm:grid-cols-2">
              <InfoRow labelKey="users.displayName" value={user.display_name} />
              <InfoRow labelKey="users.firstName" value={user.first_name} />
              <InfoRow labelKey="users.lastName" value={user.last_name} />
              <InfoRow labelKey="users.titleField" value={user.title} />
              <InfoRow labelKey="users.language" value={user.language} />
              <InfoRow
                labelKey="users.avatar"
                value={
                  user.avatar ? (
                    <a href={user.avatar} target="_blank" rel="noreferrer" className="text-primary hover:underline">
                      {t('users.avatarOpen')}
                    </a>
                  ) : undefined
                }
              />
            </dl>
          </div>

          <div className="space-y-3">
            <h3 className="text-[13px] font-semibold tracking-tight">{t('users.sectionProfileContact')}</h3>
            <dl className="grid gap-3 sm:grid-cols-2">
              <InfoRow
                labelKey="users.email"
                value={
                  user.email ? (
                    <span className="inline-flex items-center gap-2">
                      <span className="truncate">{user.email}</span>
                      <Badge variant={user.email_verified ? 'success' : 'outline'}>
                        {user.email_verified ? t('users.verified') : t('users.unverified')}
                      </Badge>
                    </span>
                  ) : undefined
                }
              />
              <InfoRow
                labelKey="users.phone"
                value={
                  user.phone ? (
                    <span className="inline-flex items-center gap-2">
                      <span className="truncate">{user.phone}</span>
                      <Badge variant={user.phone_verified ? 'success' : 'outline'}>
                        {user.phone_verified ? t('users.verified') : t('users.unverified')}
                      </Badge>
                    </span>
                  ) : undefined
                }
              />
              <InfoRow labelKey="users.countryCode" value={user.country_code} />
            </dl>
          </div>

          <div className="space-y-3">
            <h3 className="text-[13px] font-semibold tracking-tight">{t('users.sectionProfileOther')}</h3>
            <dl className="grid gap-3 sm:grid-cols-2">
              <InfoRow labelKey="users.affiliation" value={user.affiliation} />
              <InfoRow labelKey="users.location" value={user.location} />
              <InfoRow labelKey="users.address" value={user.address} />
              <InfoRow labelKey="users.homepage" value={user.homepage} />
              <InfoRow labelKey="users.gender" value={user.gender} />
              <InfoRow labelKey="users.birthday" value={user.birthday} />
              <InfoRow labelKey="users.education" value={user.education} />
              <InfoRow labelKey="users.bio" value={user.bio} />
            </dl>
          </div>
        </CardContent>
      </Card>

      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('users.sectionIdentity')}</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="space-y-3">
              <InfoRow labelKey="users.name" value={user.name} />
              <InfoRow labelKey="users.uid" value={<span className="font-mono text-xs">{user.uid}</span>} />
              <InfoRow
                labelKey="users.accountType"
                value={
                  <Badge variant={user.is_admin ? 'info' : 'secondary'}>
                    {user.is_admin ? t('users.isAdmin') : t('users.typeNormal')}
                  </Badge>
                }
              />
              <InfoRow
                labelKey="users.registerSource"
                value={user.register_source || user.register_type || t('users.registerSourceLocal')}
              />
              <InfoRow
                labelKey="users.verifiedState"
                value={t('users.verifiedSummary', { count: verifiedCount })}
              />
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('users.sectionBelonging')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <dl className="space-y-3">
              <InfoRow labelKey="users.department" value={user.department_path} />
              <InfoRow
                labelKey="users.lastSignin"
                value={user.last_signin_time ? formatRelative(user.last_signin_time) : t('users.neverSignin')}
              />
              <InfoRow
                labelKey="users.lastSigninIP"
                value={user.last_signin_ip ? <span className="font-mono text-xs">{user.last_signin_ip}</span> : undefined}
              />
              <InfoRow labelKey="users.createdAt" value={formatDateTime(user.created_at)} />
              <InfoRow labelKey="users.updatedAt" value={formatDateTime(user.updated_at)} />
            </dl>
            <p className="text-muted-foreground text-xs">{t('users.belongingReadonlyHint')}</p>
            {user.is_forbidden ? <Badge variant="destructive">{t('users.isForbidden')}</Badge> : null}
            {user.is_deleted ? <Badge variant="destructive">{t('users.isDeleted')}</Badge> : null}
            {user.is_online ? <Badge variant="success">{t('users.isOnline')}</Badge> : null}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
