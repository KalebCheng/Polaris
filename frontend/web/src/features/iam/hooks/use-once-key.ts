import { useEffect, useRef } from 'react'

/**
 * 按 key 只加载一次的副作用守卫。
 *
 * 为什么需要它：React StrictMode 在开发环境会双调用 effect，若直接在 effect 里发请求，
 * 每个接口都会被打两次（第四阶段"打开登录页 status 被请求 4 次"、第五阶段"用户详情标签重复请求"
 * 都是同一个根因）。这里用 ref 记录**最近一次已加载的 key**：
 * key 相同 → 不再触发；key 变化（换了用户、翻了页）→ 正常重新加载。
 */
export function useOnceKey(key: string, loader: () => void | Promise<void>) {
  const loadedKey = useRef<string>('')
  const loaderRef = useRef(loader)
  loaderRef.current = loader

  useEffect(() => {
    if (!key || loadedKey.current === key) return
    loadedKey.current = key
    void loaderRef.current()
  }, [key])
}
