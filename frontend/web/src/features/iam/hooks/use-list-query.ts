import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/shared/ui/toast'
import type { PageData } from '@/shared/api/types'
import { describeError } from '@/shared/lib/errors'
import type { ListParams } from '../api'

type ListFetcherParams = ListParams & Record<string, string | number | boolean | undefined>
type Fetcher<T> = (params: ListFetcherParams) => Promise<PageData<T>>

interface UseListQueryOptions {
  /** 组织维度收敛（组织内资源传当前组织） */
  institution?: string
  /** 平台级（跨组织）列表：传 'platform' */
  scope?: 'platform'
  size?: number
  /**
   * 附加筛选参数：既随请求发送，也参与"在途去重"键（改变即重新加载，并回到第 1 页）。
   * 用于部门 / 状态 / 是否绑定 MFA 这类目录筛选。
   */
  filters?: ListFetcherParams
}

/**
 * 列表查询通用逻辑：分页、关键字检索、组织/平台范围、加载与错误处理。
 * 关键字输入即触发（防抖 300ms），也可通过 search() 立即提交。
 * 各资源页只需提供 fetcher。
 */
export function useListQuery<T>(fetcher: Fetcher<T>, options?: UseListQueryOptions) {
  const { t } = useTranslation()
  const [items, setItems] = useState<T[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [appliedKeyword, setAppliedKeyword] = useState('')
  const [loading, setLoading] = useState(true)

  // 默认每页 15 条：长列表页首屏能看到更多，又不会让表格长得离谱
  const size = options?.size ?? 15
  const institution = options?.institution
  const scope = options?.scope
  const filtersKey = JSON.stringify(options?.filters ?? {})
  // 用 ref 持有"最新筛选值"：filtersKey 变化时才重排请求，避免调用方内联对象导致每次渲染都重新请求
  const filtersRef = useRef(options?.filters)
  filtersRef.current = options?.filters

  // 防抖：输入停止 300ms 后应用到查询
  const firstRender = useRef(true)
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    const timer = setTimeout(() => {
      setPage(1)
      setAppliedKeyword(keyword.trim())
    }, 300)
    return () => clearTimeout(timer)
  }, [keyword])

  // 筛选条件变化 → 回到第 1 页（否则会出现"筛完还在第 3 页，结果是空的"）
  const filtersFirstRender = useRef(true)
  useEffect(() => {
    if (filtersFirstRender.current) {
      filtersFirstRender.current = false
      return
    }
    setPage(1)
  }, [filtersKey])

  // 同一实例、同一参数的**在途**请求只发一次：React 严格模式下挂载 effect 会执行两次，
  // 直接复用进行中的 Promise，避免"进页面就发两条相同分页请求"；请求结束后清空，
  // 因此显式 reload()（增删改后刷新）仍然会真正发起新请求。
  const inflight = useRef<{ key: string; promise: Promise<void> } | null>(null)

  const load = useCallback(async () => {
    const key = `${institution ?? ''}|${scope ?? ''}|${appliedKeyword}|${page}|${size}|${filtersKey}`
    if (inflight.current?.key === key) return inflight.current.promise

    const request = (async () => {
      setLoading(true)
      try {
        const result = await fetcher({
          institution,
          scope,
          keyword: appliedKeyword,
          page,
          size,
          ...filtersRef.current,
        })
        setItems(result.items ?? [])
        setTotal(result.total ?? 0)
      } catch (error) {
        setItems([])
        setTotal(0)
        toast.error(describeError(error, t))
      } finally {
        setLoading(false)
        if (inflight.current?.key === key) inflight.current = null
      }
    })()
    inflight.current = { key, promise: request }
    return request
    // fetcher 由调用方内联定义，此处按参数变化触发即可
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [institution, scope, appliedKeyword, page, size, filtersKey, t])

  useEffect(() => {
    void load()
  }, [load])

  const search = useCallback(() => {
    setPage(1)
    setAppliedKeyword(keyword.trim())
  }, [keyword])

  return {
    items,
    total,
    page,
    size,
    keyword,
    loading,
    setKeyword,
    setPage,
    search,
    reload: load,
  }
}
