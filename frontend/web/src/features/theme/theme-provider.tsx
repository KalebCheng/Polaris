import { ThemeProvider as NextThemesProvider } from 'next-themes'

/** 主题 Provider：light / dark / system 三态，持久化到 localStorage */
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  return (
    <NextThemesProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      {children}
    </NextThemesProvider>
  )
}
