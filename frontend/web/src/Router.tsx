import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Spinner } from '@/shared/ui'
import { AppLayout } from '@/widgets/layout/app-layout'
import { PlatformLayout } from '@/widgets/layout/platform-layout'
import { WorkspaceLayout } from '@/widgets/layout/workspace-layout'
import { RequireAuth } from '@/features/auth/require-auth'
import { RequireConsoleAccess } from '@/features/auth/require-console-access'
import { PortalLayout } from '@/features/portal/portal-layout'

// 红线：所有页面按路由懒加载，避免首屏 bundle 膨胀
const LoginPage = lazy(() => import('@/features/auth/login-page'))
const OAuthCallbackPage = lazy(() => import('@/features/auth/oauth-callback-page'))
// 托管注册页与魔法链接落地页（公开，按 client_id 品牌化）
const SignupPage = lazy(() => import('@/features/auth/signup-page'))
const MagicLinkPage = lazy(() => import('@/features/auth/magic-link-page'))
const DashboardPage = lazy(() => import('@/pages/dashboard-page'))
const InstitutionsPage = lazy(() => import('@/features/iam/pages/institutions-page'))
const InstitutionEditPage = lazy(() => import('@/features/iam/pages/institution-edit-page'))
const InstitutionOverviewPage = lazy(() => import('@/pages/institution-detail-page'))
const UsersPage = lazy(() => import('@/features/iam/pages/users-page'))
const UserDetailPage = lazy(() => import('@/features/iam/pages/user-detail-page'))
const UserEditPage = lazy(() => import('@/features/iam/pages/user-edit-page'))
const GroupsPage = lazy(() => import('@/features/iam/pages/groups-page'))
const GroupDetailPage = lazy(() => import('@/features/iam/pages/group-detail-page'))
const DepartmentsPage = lazy(() => import('@/features/iam/pages/departments-page'))
const ApplicationsPage = lazy(() => import('@/features/iam/pages/applications-page'))
const ApplicationEditPage = lazy(() => import('@/features/iam/pages/application-edit-page'))
const ProvidersPage = lazy(() => import('@/features/iam/pages/providers-page'))
const ProviderEditPage = lazy(() => import('@/features/iam/pages/provider-edit-page'))
const ConnectionsPage = lazy(() => import('@/features/iam/pages/connections-page'))
const AsyncdPage = lazy(() => import('@/features/iam/pages/asyncd-page'))
const ConnectionEditPage = lazy(() => import('@/features/iam/pages/connection-edit-page'))
const AccountStrategiesPage = lazy(() => import('@/features/iam/pages/account-strategies-page'))
const AppAccountsPage = lazy(() => import('@/features/iam/pages/app-accounts-page'))
const ProvisioningPage = lazy(() => import('@/features/iam/pages/provisioning-page'))
// 平台级（跨组织）
const RolesPage = lazy(() => import('@/features/iam/pages/roles-page'))
const PermissionsPage = lazy(() => import('@/features/iam/pages/permissions-page'))
const CertsPage = lazy(() => import('@/features/iam/pages/certs-page'))
const StorageSettingsPage = lazy(() => import('@/features/iam/pages/storage-settings-page'))
const TokensPage = lazy(() => import('@/features/iam/pages/tokens-page'))
const SessionsPage = lazy(() => import('@/features/iam/pages/sessions-page'))
const RecordsPage = lazy(() => import('@/features/iam/pages/records-page'))
const PortalAppsPage = lazy(() => import('@/features/portal/pages/apps-page'))
const PortalProfilePage = lazy(() => import('@/features/portal/pages/profile-page'))
const PortalSecurityPage = lazy(() => import('@/features/portal/pages/security-page'))
const PortalIdentitiesPage = lazy(() => import('@/features/portal/pages/identities-page'))
const PortalAppAccountsPage = lazy(() => import('@/features/portal/pages/app-accounts-page'))
const PortalSessionsPage = lazy(() => import('@/features/portal/pages/sessions-page'))
const PortalActivityPage = lazy(() => import('@/features/portal/pages/activity-page'))

function RouteFallback() {
  const { t } = useTranslation()
  return (
    <div className="flex h-full items-center justify-center gap-2">
      <Spinner />
      <span className="text-muted-foreground text-sm">{t('common.loading')}</span>
    </div>
  )
}

/**
 * 兼容旧地址（无组织前缀的资源路径）：重定向到"上次访问的组织"下的同名页面。
 * 组织内资源统一挂在 /institutions/:inst/... 下。
 */
function LegacyRedirect({ suffix }: { suffix: string }) {
  const location = useLocation()
  const institution = window.localStorage.getItem('polaris.institution') ?? ''
  if (!institution) return <Navigate to="/institutions" replace />
  return <Navigate to={`/institutions/${institution}${suffix}${location.search}`} replace />
}

/** 旧前缀 /organizations/* → /institutions/*（保留其余路径与查询串，永久兼容） */
function OrganizationsRedirect() {
  const location = useLocation()
  const rest = location.pathname.replace(/^\/organizations/, '')
  return <Navigate to={`/institutions${rest}${location.search}`} replace />
}

export function Router() {
  return (
    <Suspense fallback={<RouteFallback />}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        {/* OAuth 回调中间页：支持根路径与 provider 子路径两种注册方式 */}
        <Route path="/oauth/callback" element={<OAuthCallbackPage />} />
        <Route path="/oauth/callback/:provider" element={<OAuthCallbackPage />} />
        {/* 终端用户自助注册 / 邮件魔法链接落地 */}
        <Route path="/signup" element={<SignupPage />} />
        <Route path="/magic" element={<MagicLinkPage />} />

        <Route element={<RequireAuth />}>
          <Route path="/portal" element={<PortalLayout />}>
            <Route index element={<PortalAppsPage />} />
            <Route path="profile" element={<PortalProfilePage />} />
            <Route path="security" element={<PortalSecurityPage />} />
            <Route path="identities" element={<PortalIdentitiesPage />} />
            <Route path="app-accounts" element={<PortalAppAccountsPage />} />
            <Route path="sessions" element={<PortalSessionsPage />} />
            <Route path="activity" element={<PortalActivityPage />} />
          </Route>

          <Route element={<RequireConsoleAccess />}>
            <Route element={<AppLayout />}>
              {/* 平台级（跨组织）：总览 / 组织 / 角色 / 权限 / 安全管理 / 审计 */}
              <Route element={<PlatformLayout />}>
                <Route index element={<DashboardPage />} />
                <Route path="/institutions" element={<InstitutionsPage />} />
                <Route path="/institutions/new" element={<InstitutionEditPage />} />
                <Route path="/institutions/:id/edit" element={<InstitutionEditPage />} />
                <Route path="/roles" element={<RolesPage />} />
                <Route path="/permissions" element={<PermissionsPage />} />
                <Route path="/security" element={<Navigate to="/security/certs" replace />} />
                <Route path="/security/certs" element={<CertsPage />} />
                <Route path="/security/tokens" element={<TokensPage />} />
                <Route path="/security/sessions" element={<SessionsPage />} />
                <Route path="/audit" element={<RecordsPage />} />
                <Route path="/asyncd" element={<AsyncdPage />} />
                <Route path="/settings" element={<Navigate to="/settings/storage" replace />} />
                <Route path="/settings/storage" element={<StorageSettingsPage />} />
              </Route>

              {/* 组织内（工作区）：左侧导航 + 内容 */}
              <Route path="/institutions/:inst" element={<WorkspaceLayout />}>
                <Route index element={<InstitutionOverviewPage />} />
                <Route path="departments" element={<DepartmentsPage />} />
                <Route path="users" element={<UsersPage />} />
                <Route path="users/new" element={<UserEditPage />} />
                <Route path="users/:id" element={<UserDetailPage />} />
                <Route path="users/:id/edit" element={<UserEditPage />} />
                <Route path="groups" element={<GroupsPage />} />
                <Route path="groups/:id" element={<GroupDetailPage />} />
                <Route path="applications" element={<ApplicationsPage />} />
                <Route path="applications/new" element={<ApplicationEditPage />} />
                <Route path="applications/:id" element={<ApplicationEditPage />} />
                <Route path="providers" element={<ProvidersPage />} />
                <Route path="providers/new" element={<ProviderEditPage />} />
                <Route path="providers/:id" element={<ProviderEditPage />} />
                <Route path="connections" element={<ConnectionsPage />} />
                <Route path="connections/new" element={<ConnectionEditPage />} />
                <Route path="connections/:id" element={<ConnectionEditPage />} />
                <Route path="account-strategies" element={<AccountStrategiesPage />} />
                <Route path="app-accounts" element={<AppAccountsPage />} />
                <Route path="provisioning" element={<ProvisioningPage />} />
              </Route>

              {/* 旧前缀 /organizations/* → /institutions/*（永久兼容，ADR D1） */}
              <Route path="/organizations" element={<OrganizationsRedirect />} />
              <Route path="/organizations/*" element={<OrganizationsRedirect />} />

              {/* 旧地址兼容：无组织前缀的"组织内资源"路径 */}
              <Route path="/users" element={<LegacyRedirect suffix="/users" />} />
              <Route path="/groups" element={<LegacyRedirect suffix="/groups" />} />
              <Route path="/departments" element={<LegacyRedirect suffix="/departments" />} />
              <Route path="/applications" element={<LegacyRedirect suffix="/applications" />} />
              <Route path="/applications/new" element={<LegacyRedirect suffix="/applications/new" />} />
              <Route path="/providers" element={<LegacyRedirect suffix="/providers" />} />
              <Route path="/syncers" element={<LegacyRedirect suffix="/connections?direction=in" />} />
              <Route path="/connectors" element={<LegacyRedirect suffix="/connections?direction=out" />} />
              <Route path="/account-strategies" element={<LegacyRedirect suffix="/account-strategies" />} />
              <Route path="/app-accounts" element={<LegacyRedirect suffix="/app-accounts" />} />
              <Route path="/provisioning" element={<LegacyRedirect suffix="/provisioning" />} />

              {/* 旧地址兼容：安全资源已提升为平台级「安全管理」 */}
              <Route path="/certs" element={<Navigate to="/security/certs" replace />} />
              <Route path="/tokens" element={<Navigate to="/security/tokens" replace />} />
              <Route path="/sessions" element={<Navigate to="/security/sessions" replace />} />
              <Route path="/records" element={<Navigate to="/audit" replace />} />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  )
}
