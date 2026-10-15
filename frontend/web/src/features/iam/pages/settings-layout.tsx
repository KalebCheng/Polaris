import { settingsTabs } from '@/widgets/layout/nav-config'
import { SubnavPageLayout } from './subnav-page-layout'

/**
 * 系统设置（存储设置 / 任务中心 / 审计日志）：页头与二级导航由组级布局承载。
 *
 * 二级导航用**纵向菜单**（对齐组织管理的资源侧栏）：这三个是彼此独立的运维页面，
 * 横向胶囊行会把页头挤成一条，且每页都从"标题"重新开始，看不出"这一组里有哪几页、我在哪一页"。
 */
export function SettingsLayout() {
  return <SubnavPageLayout tabs={settingsTabs} ariaLabelKey="nav.settings" variant="menu" />
}
