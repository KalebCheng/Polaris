import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Spinner } from '@/shared/ui'
import { AppLayout } from '@/widgets/layout/app-layout'
import { PlatformLayout } from '@/widgets/layout/platform-layout'
import { SecurityLayout } from '@/features/iam/pages/security-layout'
import { SettingsLayout } from '@/features/iam/pages/settings-layout'
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
const JobTitlesPage = lazy(() => import('@/features/iam/pages/job-titles-page'))
const JobLevelsPage = lazy(() => import('@/features/iam/pages/job-levels-page'))
const JobTitleDetailPage = lazy(() => import('@/features/iam/pages/job-title-detail-page'))
const DepartmentsPage = lazy(() => import('@/features/iam/pages/departments-page'))
const ApplicationsPage = lazy(() => import('@/features/iam/pages/applications-page'))
const ApplicationEditPage = lazy(() => import('@/features/iam/pages/application-edit-page'))
const ProvidersPage = lazy(() => import('@/features/iam/pages/providers-page'))
const ProviderEditPage = lazy(() => import('@/features/iam/pages/provider-edit-page'))
const ConnectionsPage = lazy(() => import('@/features/iam/pages/connections-page'))
const AsyncdPage = lazy(() => import('@/features/iam/pages/asyncd-page'))
const ConnectionEditPage = lazy(() => import('@/features/iam/pages/connection-edit-page'))
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
const PortalAppAccountsPage = lazy(() => import('@/features/portal/pages/application-accounts-page'))
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
        {/* SSO 回调中间页：挂在前端自己的命名空间 /sso/**，与后端的 OIDC 协议路径 /oauth/* 彻底分开
            （反代层因此不需要为某个前端页面开"前缀例外"） */}
        <Route path="/sso/callback" element={<OAuthCallbackPage />} />
        <Route path="/sso/callback/:provider" element={<OAuthCallbackPage />} />
        {/* 终端用户自助注册 / 邮件魔法链接落地 */}
        <Route path="/signup" element={<SignupPage />} />
        <Route path="/magic" element={<MagicLinkPage />} />

        <Route element={<RequireAuth />}>
          <Route path="/portal" element={<PortalLayout />}>
            <Route index element={<PortalAppsPage />} />
            <Route path="profile" element={<PortalProfilePage />} />
            <Route path="security" element={<PortalSecurityPage />} />
            <Route path="identities" element={<PortalIdentitiesPage />} />
            <Route path="application-accounts" element={<PortalAppAccountsPage />} />
            <Route path="sessions" element={<PortalSessionsPage />} />
            <Route path="activity" element={<PortalActivityPage />} />
          </Route>

          <Route element={<RequireConsoleAccess />}>
            <Route element={<AppLayout />}>
              {/* 平台级（跨组织）：总览 / 组织 / 角色 / 权限 / 安全管理 / 设置 */}
              <Route element={<PlatformLayout />}>
                <Route index element={<DashboardPage />} />
                <Route path="/institutions" element={<InstitutionsPage />} />
                <Route path="/institutions/new" element={<InstitutionEditPage />} />
                <Route path="/institutions/:id/edit" element={<InstitutionEditPage />} />
                <Route path="/roles" element={<RolesPage />} />
                <Route path="/permissions" element={<PermissionsPage />} />
                {/*
                  安全管理 / 系统设置：页头（标题 / 描述 / 操作）与二级标签由**组级布局**承载，
                  页面只提供内容 —— 标签因此在路由切换中不卸载，才能像顶栏导航那样"滑"过去
                  （各页自己渲染标签时每次都是新元素，只能瞬时落位，见 docs/design-language.md §5.0）。
                */}
                <Route path="/security" element={<SecurityLayout />}>
                  <Route index element={<Navigate to="certs" replace />} />
                  <Route path="certs" element={<CertsPage />} />
                  <Route path="tokens" element={<TokensPage />} />
                  <Route path="sessions" element={<SessionsPage />} />
                </Route>
                <Route path="/settings" element={<SettingsLayout />}>
                  <Route index element={<Navigate to="storage" replace />} />
                  <Route path="storage" element={<StorageSettingsPage />} />
                  <Route path="tasks" element={<AsyncdPage />} />
                  <Route path="audit" element={<RecordsPage />} />
                </Route>
                {/* 旧地址：任务中心与审计日志已成为设置的二级页，永久兼容 */}
                <Route path="/asyncd" element={<Navigate to="/settings/tasks" replace />} />
                <Route path="/audit" element={<Navigate to="/settings/audit" replace />} />
              </Route>

              {/* 组织内（工作区）：左侧导航 + 内容 */}
              <Route path="/institutions/:inst" element={<WorkspaceLayout />}>
                <Route index element={<InstitutionOverviewPage />} />
                <Route path="departments" element={<DepartmentsPage />} />
                <Route path="users" element={<UsersPage />} />
                <Route path="users/new" element={<UserEditPage />} />
                <Route path="users/:id" element={<UserDetailPage />} />
                <Route path="users/:id/edit" element={<UserEditPage />} />
                <Route path="job-titles" element={<JobTitlesPage />} />
                <Route path="job-levels" element={<JobLevelsPage />} />
                <Route path="job-titles/:id" element={<JobTitleDetailPage />} />
                <Route path="applications" element={<ApplicationsPage />} />
                <Route path="applications/new" element={<ApplicationEditPage />} />
                <Route path="applications/:id" element={<ApplicationEditPage />} />
                <Route path="providers" element={<ProvidersPage />} />
                <Route path="providers/new" element={<ProviderEditPage />} />
                <Route path="providers/:id" element={<ProviderEditPage />} />
                <Route path="connections" element={<ConnectionsPage />} />
                <Route path="connections/new" element={<ConnectionEditPage />} />
                <Route path="connections/:id" element={<ConnectionEditPage />} />
                <Route path="account-strategies" element={<Navigate to="../applications" replace />} />
                <Route path="application-accounts" element={<Navigate to="../applications" replace />} />
                <Route path="provisioning" element={<Navigate to="../applications" replace />} />
              </Route>

              {/* 旧前缀 /organizations/* → /institutions/*（永久兼容，ADR D1） */}
              <Route path="/organizations" element={<OrganizationsRedirect />} />
              <Route path="/organizations/*" element={<OrganizationsRedirect />} />

              {/* 旧地址兼容：无组织前缀的"组织内资源"路径 */}
              <Route path="/users" element={<LegacyRedirect suffix="/users" />} />
              <Route path="/job-titles" element={<LegacyRedirect suffix="/job-titles" />} />
              <Route path="/departments" element={<LegacyRedirect suffix="/departments" />} />
              <Route path="/applications" element={<LegacyRedirect suffix="/applications" />} />
              <Route path="/applications/new" element={<LegacyRedirect suffix="/applications/new" />} />
              <Route path="/providers" element={<LegacyRedirect suffix="/providers" />} />
              <Route path="/syncers" element={<LegacyRedirect suffix="/connections?direction=in" />} />
              <Route path="/connectors" element={<LegacyRedirect suffix="/connections?direction=out" />} />
              <Route path="/account-strategies" element={<LegacyRedirect suffix="/applications" />} />
              <Route path="/application-accounts" element={<LegacyRedirect suffix="/applications" />} />
              <Route path="/provisioning" element={<LegacyRedirect suffix="/applications" />} />

              {/* 旧地址兼容：安全资源已提升为平台级「安全管理」 */}
              <Route path="/certs" element={<Navigate to="/security/certs" replace />} />
              <Route path="/tokens" element={<Navigate to="/security/tokens" replace />} />
              <Route path="/sessions" element={<Navigate to="/security/sessions" replace />} />
              <Route path="/records" element={<Navigate to="/settings/audit" replace />} />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  )
}
