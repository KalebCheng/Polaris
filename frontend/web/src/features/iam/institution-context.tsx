import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { institutionApi } from './api'
import { setActiveInstitution } from './active-institution'
import { resolveBrandScope, type BrandScope } from './lib/brand-scope'

// 供页面/布局复用（品牌作用域解析见 lib/brand-scope.ts）
export { resolveBrandScope, type BrandScope }

interface InstitutionOption {
  id: number
  name: string
  display_name: string
  logo?: string
  favicon?: string
}

interface InstitutionContextValue {
  institutions: InstitutionOption[]
  /** 当前组织主键 id（字符串，便于拼 URL 与查询串）：URL、链接与 API 一律用它 */
  institution: string
  /** institution 的别名（语义更明确处使用） */
  institutionId: string
  /** 当前上下文（"上次访问"）组织 Logo —— 用户门户按它取品牌 */
  logo: string
  /** 当前上下文（"上次访问"）组织 Favicon —— 用户门户按它取标签图标 */
  favicon: string
  /** 当前页面的品牌作用域 */
  brandScope: BrandScope
  /**
   * 当前页面应展示的品牌 Logo：
   * 管理端控制台一律为空（回退平台内置矢量标识）；仅用户门户使用所属组织 Logo。
   */
  brandLogo: string
  /** 当前页面应使用的浏览器标签图标（同上口径） */
  brandFavicon: string
  setInstitution: (id: string) => void
  loading: boolean
  reload: () => Promise<void>
}

const STORAGE_KEY = 'polaris.institution'
const LEGACY_STORAGE_KEY = 'polaris.organization'

/** 判断是否为数字主键（组织段现在用 id；历史遗留值是组织名） */
function isNumericID(value: string): boolean {
  return /^\d+$/.test(value)
}

/** 读取"上次访问的租户"：兼容旧键 polaris.organization（一次性迁移，见 Spec ADR D4）。 */
function readStoredInstitution(): string {
  const current = window.localStorage.getItem(STORAGE_KEY)
  if (current) return current
  const legacy = window.localStorage.getItem(LEGACY_STORAGE_KEY)
  if (legacy) {
    window.localStorage.setItem(STORAGE_KEY, legacy)
    window.localStorage.removeItem(LEGACY_STORAGE_KEY)
    return legacy
  }
  return ''
}

const InstitutionContext = createContext<InstitutionContextValue | null>(null)

/**
 * 当前租户上下文：把"租户"提升为控制台的一等维度。
 * 侧边栏顶部切换租户后，各资源页（用户/部门/应用/角色/权限/提供方…）统一收敛到该租户，
 * 租户详情页也会把上下文切到目标租户，从而复用同一套资源页。
 */
export function InstitutionProvider({
  children,
  /** 仅在已认证时拉取租户列表：登录页不应请求受保护的租户接口 */
  enabled = true,
}: {
  children: React.ReactNode
  enabled?: boolean
}) {
  const [institutions, setInstitutions] = useState<InstitutionOption[]>([])
  // 存的是组织主键 id；历史用户存的可能是组织名，加载列表后按名字回填 id
  const [selection, setSelectionState] = useState<string>(readStoredInstitution)
  const [loading, setLoading] = useState(enabled)

  const reload = useCallback(async () => {
    // 拉取期间必须显式置 loading：否则消费者（如工作区布局）会把"还没拿到列表"
    // 误判成"该组织不存在"，于是刷新 / 深链接一进来就被弹回组织列表。
    setLoading(true)
    try {
      const result = await institutionApi.list({ size: 200 })
      const options = result.items.map((item) => ({
        id: item.id,
        name: item.name,
        display_name: item.display_name,
        logo: item.logo,
        favicon: item.favicon,
      }))
      setInstitutions(options)
      setSelectionState((current) => {
        // 已是 id 且仍存在 → 保留
        if (current && options.some((item) => String(item.id) === current)) return current
        // 兼容历史：存的是组织名 → 找到对应 id 并落盘
        const byName = options.find((item) => item.name === current)
        if (byName) {
          window.localStorage.setItem(STORAGE_KEY, String(byName.id))
          return String(byName.id)
        }
        const first = options[0]?.id
        return first ? String(first) : ''
      })
    } catch {
      setInstitutions([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!enabled) {
      setLoading(false)
      return
    }
    void reload()
  }, [enabled, reload])

  const current = useMemo(
    () => institutions.find((item) => String(item.id) === selection || item.name === selection),
    [institutions, selection],
  )
  // 当前组织 id（URL/查询串一律用它）；列表就绪后归一化为真实主键
  const institution = current ? String(current.id) : isNumericID(selection) ? selection : ''

  // 把"当前工作区组织"同步到 api 层，使其自动注入到组织内请求的 ?institution=（见 active-institution.ts）
  useEffect(() => {
    setActiveInstitution(institution)
  }, [institution])

  const setInstitution = useCallback((id: string) => {
    setSelectionState(id)
    window.localStorage.setItem(STORAGE_KEY, id)
  }, [])

  const logo = current?.logo ?? ''
  const favicon = current?.favicon ?? ''

  // 品牌按"当前路径"取：管理端控制台一律用平台品牌；用户门户用所属组织品牌
  const { pathname } = useLocation()
  const brandScope = useMemo(() => resolveBrandScope(pathname), [pathname])
  const brandLogo = brandScope === 'tenant' ? logo : ''
  const brandFavicon = brandScope === 'tenant' ? favicon : ''

  const value = useMemo<InstitutionContextValue>(
    () => ({
      institutions,
      institution,
      institutionId: institution,
      logo,
      favicon,
      brandScope,
      brandLogo,
      brandFavicon,
      setInstitution,
      loading,
      reload,
    }),
    [
      institutions,
      institution,
      logo,
      favicon,
      brandScope,
      brandLogo,
      brandFavicon,
      setInstitution,
      loading,
      reload,
    ],
  )
  return (
    <InstitutionContext.Provider value={value}>
      {/* React 19 会把 <link> 提升到 <head>：
          管理端不注入（回退 index.html 的平台品牌图标）；仅用户门户使用所属组织 favicon */}
      {brandFavicon ? <link rel="icon" href={brandFavicon} /> : null}
      {children}
    </InstitutionContext.Provider>
  )
}

export function useCurrentInstitution() {
  const ctx = useContext(InstitutionContext)
  if (!ctx) throw new Error('useCurrentInstitution 必须在 InstitutionProvider 内使用')
  return ctx
}

/**
 * 工作区组织**名称**：资源页的 API 调用（`?institution=` 与请求体）都按组织名收敛，
 * 而 URL（`/institutions/:inst`）承载的是组织**主键 id**——二者由上下文完成映射。
 *
 * 为什么需要映射：所有业务表都以组织名作租户键（外键指向 institutions(name)），
 * 把库里全面改成 id 需要一次全库迁移；因此 URL 用 id（稳定、可分享），API 仍用名称。
 */
export function useWorkspaceInstitution(): string {
  return useCurrentInstitution().institution
}

/** 工作区组织主键 id（URL 段）：用于拼站内链接 */
export function useWorkspaceInstitutionId(): string {
  return useCurrentInstitution().institutionId
}
