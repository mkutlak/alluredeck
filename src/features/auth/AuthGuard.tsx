import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/store/auth'

interface AuthGuardProps {
  children: React.ReactNode
}

export function AuthGuard({ children }: AuthGuardProps) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const expiresAt = useAuthStore((s) => s.expiresAt)
  const location = useLocation()

  const sessionValid = isAuthenticated && (expiresAt === null || expiresAt > Date.now())

  if (!sessionValid) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}
