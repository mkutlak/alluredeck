import { ThemeProvider as NextThemesProvider } from 'next-themes'
import type { ThemeProviderProps } from 'next-themes'

export function ThemeProvider({ children, ...props }: ThemeProviderProps) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      // This SPA never server-renders, so React never runs the provider's no-flash <script> and
      // warns about it in dev. A non-JS type makes it an inert data block, which React accepts.
      scriptProps={{ type: 'application/json' }}
      {...props}
    >
      {children}
    </NextThemesProvider>
  )
}
