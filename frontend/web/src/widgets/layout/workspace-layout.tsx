import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate, useParams } from 'react-router-dom'
import { Spinner } from '@/shared/ui'
import { useCurrentInstitution } from '@/features/iam/institution-context'
import { AppSidebar } from './app-sidebar'

/**
 * 工作区布局：左侧当前组织的资源导航 + 右侧内容区（整幅铺满）。
 *
 * 地址栏里的组织段是**组织主键 id**（`/institutions/:id/...`）。这里负责：
 * 1. 把 URL 的 id 同步进上下文（URL 是唯一事实来源）；
 * 2. 兼容历史地址（按组织名）——就地重定向到 id 版本；
 * 3. 在组织解析完成前不渲染内容——否则组织名未知，后端会回退到"会话所在组织"，
 *    可能先拉取到另一个组织的数据（平台管理员跨组织操作时尤其明显）。
 */
export function WorkspaceLayout() {
  const { inst = '' } = useParams()
  const { institutions, institutionId, setInstitution, loading } = useCurrentInstitution()
  const navigate = useNavigate()
  const { pathname, search, hash } = useLocation()
  const isID = /^\d+$/.test(inst)

  // 历史地址兼容：/institutions/<组织名>/... → /institutions/<id>/...
  useEffect(() => {
    if (!inst || isID || loading) return
    const matched = institutions.find((item) => item.name === inst)
    if (!matched) return
    // 注意：pathname 是 URL 编码后的串，不能按解码后组织名的字符长度切片
    const rest = pathname.replace(/^\/institutions\/[^/]+/, '')
    navigate(`/institutions/${matched.id}${rest}${search}${hash}`, { replace: true })
  }, [inst, isID, loading, institutions, navigate, pathname, search, hash])

  // URL 是唯一事实来源：把组织 id 同步进上下文
  useEffect(() => {
    if (isID && inst !== institutionId) setInstitution(inst)
  }, [isID, inst, institutionId, setInstitution])

  // 组织不存在（**列表确实已就绪**且查不到）→ 回到组织列表。
  // 注意 `institutions.length > 0`：列表为空既可能是"还没加载完"也可能是"请求失败"，
  // 两种情况都不能判成"组织不存在"——否则刷新 / 直接打开工作区深链会被立刻弹回组织列表
  // （真实缺陷：`/institutions/:inst/...` 深链接在首次加载时失效）。
  const missing =
    !loading && isID && institutions.length > 0 && !institutions.some((item) => String(item.id) === inst)
  useEffect(() => {
    if (missing) navigate('/institutions', { replace: true })
  }, [missing, navigate])

  const ready = isID && institutionId === inst && !loading && !missing
  if (!ready) {
    return (
      <div className="flex min-w-0 flex-1 items-center justify-center py-16">
        <Spinner />
      </div>
    )
  }

  return (
    <>
      <AppSidebar />
      <main className="min-w-0 flex-1 overflow-y-auto">
        <div className="min-h-full w-full px-4 py-5 sm:px-6 sm:py-6 lg:px-8">
          <Outlet />
        </div>
      </main>
    </>
  )
}
