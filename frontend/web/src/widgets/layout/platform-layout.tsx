import { Outlet } from 'react-router-dom'

/** 平台级页面内容区：无侧边栏，整幅铺满（宽度随视口伸展，高度至少充满内容区） */
export function PlatformLayout() {
  return (
    <main className="min-w-0 flex-1 overflow-y-auto">
      <div className="min-h-full w-full px-4 py-5 sm:px-6 sm:py-6 lg:px-8">
        <Outlet />
      </div>
    </main>
  )
}
