import { useEffect, useState } from 'react'
import { institutionApi } from '../api'

export interface InstitutionOption {
  name: string
  display_name: string
}

/** 组织下拉选项：多个列表页共用 */
export function useInstitutionOptions(enabled = true) {
  const [options, setOptions] = useState<InstitutionOption[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    setLoading(true)
    void (async () => {
      try {
        const result = await institutionApi.list({ size: 100 })
        if (cancelled) return
        setOptions(result.items.map((item) => ({ name: item.name, display_name: item.display_name })))
      } catch {
        if (!cancelled) setOptions([])
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [enabled])

  return { options, loading }
}
