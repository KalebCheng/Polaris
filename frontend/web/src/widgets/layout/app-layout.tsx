import { Outlet } from 'react-router-dom'
import { TopBar } from './top-bar'

/**
 * 控制台外壳：浅蓝灰画布 + 顶部平台级导航常驻；下方为内容区。
 * 内容区由平台级布局（PlatformLayout）或工作区布局（WorkspaceLayout）承载。
 */
export function AppLayout() {
  return (
    <div className="app-canvas flex h-full flex-col">
      <TopBar />
      <div className="flex min-h-0 flex-1">
        <Outlet />
      </div>
    </div>
  )
}
