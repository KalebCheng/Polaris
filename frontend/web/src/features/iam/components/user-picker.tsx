import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MultiSelect, type SelectOption } from '@/shared/ui'
import { userApi } from '../api'

interface UserPickerProps {
  /** Selected user primary keys. */
  value: number[]
  onChange: (next: number[]) => void
  /** 当前工作区组织（列表查询需要） */
  institution?: string
  placeholder?: string
  disabled?: boolean
}

const PAGE_SIZE = 50

/**
 * 用户选择器（远程搜索多选）。
 *
 * 为什么需要它：组织成员可能上千，前端不可能一次拉全；这里把关键词交给后端分页查询，
 * 只展示前 50 条；已选项以用户主键保存，名称只用于展示。
 */
export function UserPicker({ value, onChange, institution, placeholder, disabled }: UserPickerProps) {
  const { t } = useTranslation()
  const [options, setOptions] = useState<SelectOption[]>([])

  const load = useCallback(
    async (keyword: string) => {
      try {
        const result = await userApi.list({ institution, keyword, page: 1, size: PAGE_SIZE })
        setOptions(
          (result.items ?? []).map((user) => ({
            value: String(user.id),
            label: user.display_name ? `${user.display_name}（${user.name}）` : user.name,
            hint: user.email,
          })),
        )
      } catch {
        setOptions([])
      }
    },
    [institution],
  )

  useEffect(() => {
    void load('')
  }, [load])

  return (
    <MultiSelect
      options={options}
      value={value.map(String)}
      onChange={(next) => onChange(next.map(Number))}
      onSearch={(keyword) => void load(keyword)}
      placeholder={placeholder ?? t('users.pickerPlaceholder')}
      disabled={disabled}
      maxVisible={6}
    />
  )
}
