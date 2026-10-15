import { BrowserRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/shared/i18n'
import { Toaster, TooltipProvider } from '@/shared/ui'
import { ThemeProvider } from '@/features/theme/theme-provider'
import { AuthProvider, useAuth } from '@/features/auth/auth-context'
import { InstitutionProvider } from '@/features/iam/institution-context'
import { Router } from './Router'

/** 组织列表属受保护资源：仅登录后加载（登录页改用公开的 /auth/institutions） */
function OAuthGate({ children }: { children: React.ReactNode }) {
  const { authenticated, loading } = useAuth()
  return <InstitutionProvider enabled={authenticated && !loading}>{children}</InstitutionProvider>
}

export function App() {
  return (
    <I18nextProvider i18n={i18n}>
      <ThemeProvider>
        <BrowserRouter>
          <AuthProvider>
            <OAuthGate>
              <TooltipProvider delayDuration={200}>
                <Router />
                <Toaster />
              </TooltipProvider>
            </OAuthGate>
          </AuthProvider>
        </BrowserRouter>
      </ThemeProvider>
    </I18nextProvider>
  )
}
