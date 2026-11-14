import { Button, type SelectOption } from '@/shared/ui'

/** 分段选项（用于类型 / 平台这类少量互斥选项，比下拉更直观） */
export function OptionChips({
  options,
  value,
  onChange,
}: {
  options: SelectOption[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {options.map((option) => (
        <Button
          key={option.value}
          type="button"
          size="sm"
          variant={option.value === value ? 'default' : 'outline'}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  )
}
