import { useTranslation } from 'react-i18next'
import { InstitutionSwitcher } from './institution-switcher'
import { WorkspaceNav } from './nav-list'

/**
 * 工作区侧边栏：组织切换器 + 当前组织的资源导航。
 *
 * 形态是**画布上的一块白色浮层**（与内容区卡片同一材质），而不是"贴到屏幕边的另一条色带"——
 * 侧边栏与内容区因此读起来是同一个工作台里的两栏，而不是两个应用拼在一起。
 *
 * 左 / 上留白与内容区同一套刻度（`px-4 sm:px-6 lg:px-8` + `py-5 sm:py-6`，见 top-bar.tsx 的说明）：
 * 三者的边落在同一条竖直线上，面板才不会"各浮各的"。
 */
export function AppSidebar() {
  const { t } = useTranslation()
  return (
    <aside className="hidden w-[16.5rem] shrink-0 py-5 pr-0 pl-4 sm:py-6 sm:pl-6 lg:pl-8 md:flex md:flex-col">
      <div className="bg-sidebar text-sidebar-foreground border-border/70 flex min-h-0 flex-1 flex-col rounded-2xl border shadow-soft">
        <div className="px-2 pt-3">
          <InstitutionSwitcher />
        </div>
        <p className="text-muted-foreground px-4 pt-4 pb-1 text-[10px] font-semibold tracking-wide">
          {t('nav.orgResources')}
        </p>
        <WorkspaceNav />
      </div>
    </aside>
  )
}
