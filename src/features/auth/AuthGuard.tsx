import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/store/auth'

interface AuthGuardProps {
  children: React.ReactNode
}

export function AuthGuard({ children }: AuthGuardProps) {
  const isSessionValid = useAuthStore((s) => s.isSessionValid)
  const location = useLocation()

  if (!isSessionValid()) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}
