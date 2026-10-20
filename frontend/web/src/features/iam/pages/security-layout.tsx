import { securityTabs } from '@/widgets/layout/nav-config'
import { SubnavPageLayout } from './subnav-page-layout'

/** 安全管理（证书 / 令牌 / 会话）：页头与二级标签由组级布局承载，见 subnav-page-layout.tsx */
export function SecurityLayout() {
  return <SubnavPageLayout tabs={securityTabs} ariaLabelKey="nav.security" />
}
