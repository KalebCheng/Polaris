import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui'
import { useAuth } from '@/shared/auth/auth-context'
import { AccountStrategiesPanel } from './account-strategies-panel'
import { ApplicationAccountsPanel } from './application-accounts-panel'
import { ProvisioningHistoryPanel } from './provisioning-history-panel'

interface ApplicationProvisioningProps {
  institution: string
  applicationId: number
  applicationCode: string
}

type ProvisioningTab = 'strategies' | 'accounts' | 'history'

/**
 * 应用详情 › 账号供给：把「策略 / 应用账号 / 同步记录」收进同一个应用上下文。
 *
 * 三个子页签按权限独立显示；默认落到当前用户第一个可读的页签，
 * 避免无权限的页签先发出请求再报 403。
 */
export function ApplicationProvisioning({
  institution,
  applicationId,
  applicationCode,
}: ApplicationProvisioningProps) {
  const { t } = useTranslation()
  const { can } = useAuth()
  const allowed = useMemo(() => {
    const tabs: { key: ProvisioningTab; labelKey: string }[] = []
    if (can('account_strategy', 'read')) {
      tabs.push({ key: 'strategies', labelKey: 'accountStrategies.title' })
    }
    if (can('application_account', 'read')) {
      tabs.push({ key: 'accounts', labelKey: 'appAccounts.title' })
    }
    if (can('provision_history', 'read')) {
      tabs.push({ key: 'history', labelKey: 'provisioning.title' })
    }
    return tabs
  }, [can])

  const [selected, setSelected] = useState<ProvisioningTab | ''>('')
  const keys = allowed.map((tab) => tab.key)
  const active = keys.includes(selected as ProvisioningTab) ? selected : (keys[0] ?? '')

  if (allowed.length === 0) {
    return <p className="text-muted-foreground text-sm">{t('applications.provisioningNoPermission')}</p>
  }

  return (
    <Tabs value={active} onValueChange={(value) => setSelected(value as ProvisioningTab)}>
      {allowed.length > 1 ? (
        <TabsList className="self-start">
          {allowed.map((tab) => (
            <TabsTrigger key={tab.key} value={tab.key}>
              {t(tab.labelKey)}
            </TabsTrigger>
          ))}
        </TabsList>
      ) : null}

      {can('account_strategy', 'read') ? (
        <TabsContent value="strategies" className="mt-4">
          <AccountStrategiesPanel
            institution={institution}
            applicationId={applicationId}
            applicationCode={applicationCode}
          />
        </TabsContent>
      ) : null}

      {can('application_account', 'read') ? (
        <TabsContent value="accounts" className="mt-4">
          <ApplicationAccountsPanel
            institution={institution}
            applicationId={applicationId}
            applicationCode={applicationCode}
          />
        </TabsContent>
      ) : null}

      {can('provision_history', 'read') ? (
        <TabsContent value="history" className="mt-4">
          <ProvisioningHistoryPanel
            institution={institution}
            applicationId={applicationId}
            applicationCode={applicationCode}
          />
        </TabsContent>
      ) : null}
    </Tabs>
  )
}
