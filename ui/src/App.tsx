import { useEffect, useState } from 'react'
import { BrowserRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { ThemeProvider } from '@/components/app/ThemeProvider'
import { Toaster } from '@/components/ui/toaster'
import { useAuthStore } from '@/store/auth'
import { AppRoutes } from '@/routes'

export function App() {
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: 1,
            staleTime: 30_000,
            refetchOnWindowFocus: true,
          },
        },
      }),
  )

  // Listen for 401 events emitted by the API client interceptor
  useEffect(() => {
    const handler = () => clearAuth()
    window.addEventListener('allure:unauthorized', handler)
    return () => window.removeEventListener('allure:unauthorized', handler)
  }, [clearAuth])

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <BrowserRouter>
          <AppRoutes />
          <Toaster />
        </BrowserRouter>
        {import.meta.env.DEV && <ReactQueryDevtools initialIsOpen={false} />}
      </ThemeProvider>
    </QueryClientProvider>
  )
}
