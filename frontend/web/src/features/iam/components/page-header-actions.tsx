import { createContext, useContext } from 'react'
import { createPortal } from 'react-dom'

/**
 * 页头操作宿主：由组级布局（`SubnavPageLayout`）渲染的空容器。
 *
 * 页头（标题 / 描述 / 二级标签）归布局，**操作按钮仍归页面**——按钮与页面内部状态耦合
 * （新建证书的弹窗、会话页刷新按钮的 `disabled={loading}`、存储设置编辑态的"取消 / 保存"），
 * 提到布局就得先把这些状态同步上去，稍不留意就会显示过期状态。
 *
 * 所以这里给页面一个 portal 宿主：按钮由**页面**渲染（自然跟随页面 state），
 * 只是"挂"在布局的页头里；页面卸载时 portal 一并清空，不留残留。
 */
export const PageHeaderActionsHost = createContext<HTMLDivElement | null>(null)

/** 页面侧：把页头操作按钮投放到组级布局的页头里（宿主未就绪时返回 null） */
export function usePageHeaderActions(node: React.ReactNode): React.ReactPortal | null {
  const host = useContext(PageHeaderActionsHost)
  return host ? createPortal(node, host) : null
}
