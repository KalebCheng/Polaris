import { useTranslation } from 'react-i18next'
import type { LucideIcon } from 'lucide-react'
import { NavPills, type NavEntry } from './nav-pills'

/**
 * 画布上的白色导航浮层：工作区侧栏（组织管理）与平台二级菜单（系统设置）**同一材质**。
 * 与内容区卡片是同一套"浮在画布上的面板"，不是贴边的另一条色带。
 */
export const NAV_PANEL_CLASS =
  'bg-sidebar text-sidebar-foreground border-border/70 flex min-h-0 flex-col rounded-2xl border shadow-soft'

/** 纵向菜单的一条：`labelKey` 在组件内翻译，调用方只声明"去哪、叫什么、什么图标" */
export interface NavMenuItem {
  to: string
  labelKey: string
  icon?: LucideIcon
  /** 精确匹配（总览这类前缀会命中子页的条目） */
  end?: boolean
  disabled?: boolean
  active: boolean
}

/** 可折叠分组；不给 `labelKey` 就是"没有抬头的单组"（平台二级菜单就是这种） */
export interface NavMenuSection {
  labelKey?: string
  collapsed?: boolean
  onToggle?: () => void
  items: NavMenuItem[]
}

interface NavMenuProps {
  sections: NavMenuSection[]
  /** 导航区无障碍名称（已翻译的串，如「组织导航」「系统设置」） */
  ariaLabel: string
  onNavigate?: () => void
}

/**
 * 纵向菜单——**唯一的纵向导航实现**：组织管理的资源侧栏（`WorkspaceNav`）与
 * 平台二级菜单（`SubnavPageLayout` 的 `variant="menu"`）都渲染它。
 *
 * 关键点：**整块菜单只有一个容器**（`NavPills`），分组抬头作为"不可选中的条目"一起进入列表。
 * 滑块语义是"在容器内量测选中项"，所以这样跨分组切换才会是**同一块滑块的位移**；
 * 早先每个分组各渲染一份列表，跨分组只能"旧滑块卸载、新滑块就位"，看着像没有动效。
 * 折叠/展开改变的是同一份条目数组（`NavPills` 每次提交都重新量测），下拉箭头与滑块都不会错位。
 *
 * 选中态是那块**会滑过去的墨色胶囊**（`NavPills orientation="vertical"` → `SlidingPill`）：
 * 横向（顶栏 / 二级标签）与纵向（两处菜单）共用同一份量测与令牌，只是位移轴不同。
 */
export function NavMenu({ sections, ariaLabel, onNavigate }: NavMenuProps) {
  const { t } = useTranslation()
  const entries: NavEntry[] = []
  for (const section of sections) {
    if (section.labelKey) {
      entries.push({
        kind: 'section',
        label: t(section.labelKey),
        expanded: !section.collapsed,
        onToggle: () => section.onToggle?.(),
      })
    }
    if (section.collapsed) continue
    for (const item of section.items) {
      entries.push({
        kind: 'link',
        to: item.to,
        label: t(item.labelKey),
        active: item.active,
        icon: item.icon,
        end: item.end,
        disabled: item.disabled,
      })
    }
  }
  return <NavPills orientation="vertical" ariaLabel={ariaLabel} items={entries} onNavigate={onNavigate} />
}
