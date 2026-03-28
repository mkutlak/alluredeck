import { useEffect, useRef, useState } from 'react'
import { getSession } from '@/api/auth'
import { useAuthStore } from '@/store/auth'
import type { Role } from '@/store/auth'

export function useSessionRestore(): { isRestoring: boolean } {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const roles = useAuthStore((s) => s.roles)
  const setAuth = useAuthStore((s) => s.setAuth)
  const clearAuth = useAuthStore((s) => s.clearAuth)
  // Need restoration when authenticated but roles are empty (post-refresh state)
  const needsRestore = isAuthenticated && roles.length === 0

  // useRef to prevent double-fetch in StrictMode
  const fetchingRef = useRef(false)
  const [isRestoring, setIsRestoring] = useState(needsRestore)

  useEffect(() => {
    if (!needsRestore || fetchingRef.current) return
    fetchingRef.current = true

    getSession()
      .then((res) => {
        const { roles, username, expires_in, provider } = res.data
        setAuth(roles as Role[], username, expires_in, provider)
      })
      .catch(() => {
        clearAuth()
      })
      .finally(() => {
        fetchingRef.current = false
        setIsRestoring(false)
      })
  }, [needsRestore, setAuth, clearAuth])

  return { isRestoring }
}
