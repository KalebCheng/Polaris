import { useTranslation } from 'react-i18next'
import { Languages } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger, Button } from '@/shared/ui'

/** 语言切换：中文 / English */
export function LanguageToggle() {
  const { i18n, t } = useTranslation()
  const current = i18n.resolvedLanguage ?? 'zh'

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t('language.label')}>
          <Languages className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          onClick={() => void i18n.changeLanguage('zh')}
          data-active={current.startsWith('zh')}
        >
          {t('language.zh')}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => void i18n.changeLanguage('en')}
          data-active={current.startsWith('en')}
        >
          {t('language.en')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
