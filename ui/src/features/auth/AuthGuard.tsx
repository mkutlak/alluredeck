import { useEffect } from 'react'
import { Navigate, useLocation } from 'react-router'
import { useAuthStore } from '@/store/auth'

interface AuthGuardProps {
  children: React.ReactNode
}

export function AuthGuard({ children }: AuthGuardProps) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const expiresAt = useAuthStore((s) => s.expiresAt)
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const location = useLocation()

  useEffect(() => {
    if (expiresAt === null) return
    const remaining = expiresAt - Date.now()
    if (remaining <= 0) {
      clearAuth()
      return
    }
    const timer = setTimeout(() => clearAuth(), remaining)
    return () => clearTimeout(timer)
  }, [expiresAt, clearAuth])

  // eslint-disable-next-line react-hooks/purity -- intentional: session expiry must reflect current time on each render
  const sessionValid = isAuthenticated && (expiresAt === null || expiresAt > Date.now())

  if (!sessionValid) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}
