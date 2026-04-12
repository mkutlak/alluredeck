import { useEffect } from 'react'
import { Navigate, useLocation } from 'react-router'
import { Loader2 } from 'lucide-react'
import { attemptRefresh } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { useSessionRestore } from '@/hooks/useSessionRestore'

interface AuthGuardProps {
  children: React.ReactNode
}

// Refresh the access token this many ms before its real expiry so the user
// never experiences a failed request or loading spinner mid-click.
const REFRESH_MARGIN_MS = 60 * 1000

export function AuthGuard({ children }: AuthGuardProps) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const expiresAt = useAuthStore((s) => s.expiresAt)
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const location = useLocation()

  const { isRestoring } = useSessionRestore()

  useEffect(() => {
    if (expiresAt === null) return

    let cancelled = false
    const doRefreshOrClear = async () => {
      const ok = await attemptRefresh()
      if (cancelled) return
      if (!ok) {
        clearAuth()
        return
      }
      // On success, attemptRefresh() already called setAuth() which updated
      // expiresAt, which re-triggers this effect with a fresh timer.
    }

    const remaining = expiresAt - Date.now()

    if (remaining <= REFRESH_MARGIN_MS) {
      // Already in the "about to expire" window (or past it) → refresh now.
      void doRefreshOrClear()
      return () => {
        cancelled = true
      }
    }

    // Schedule proactive refresh REFRESH_MARGIN_MS before real expiry.
    const timer = setTimeout(() => {
      void doRefreshOrClear()
    }, remaining - REFRESH_MARGIN_MS)

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [expiresAt, clearAuth])

  if (isRestoring) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Loader2 className="text-muted-foreground h-8 w-8 animate-spin" />
      </div>
    )
  }

  // eslint-disable-next-line react-hooks/purity -- intentional: session expiry must reflect current time on each render
  const sessionValid = isAuthenticated && (expiresAt === null || expiresAt > Date.now())

  if (!sessionValid) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}
