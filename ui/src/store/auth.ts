import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { setAccessToken } from '@/api/client'

export type Role = 'admin' | 'viewer'

interface AuthState {
  isAuthenticated: boolean
  roles: Role[]
  username: string | null
  expiresAt: number | null

  setAuth: (token: string, roles: Role[], username: string, expiresIn: number) => void
  clearAuth: () => void
  isAdmin: () => boolean
  isSessionValid: () => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      isAuthenticated: false,
      roles: [],
      username: null,
      expiresAt: null,

      setAuth(token, roles, username, expiresIn) {
        setAccessToken(token)
        // Mirror token in sessionStorage so it survives a same-tab page refresh
        sessionStorage.setItem('allure-token', token)
        sessionStorage.setItem('allure-token-expiry', String(Date.now() + expiresIn * 1000))
        set({
          isAuthenticated: true,
          roles,
          username,
          expiresAt: Date.now() + expiresIn * 1000,
        })
      },

      clearAuth() {
        setAccessToken(null)
        sessionStorage.removeItem('allure-token')
        sessionStorage.removeItem('allure-token-expiry')
        set({ isAuthenticated: false, roles: [], username: null, expiresAt: null })
      },

      isAdmin: () => get().roles.includes('admin'),

      isSessionValid() {
        const { isAuthenticated, expiresAt } = get()
        return isAuthenticated && (expiresAt === null || expiresAt > Date.now())
      },
    }),
    {
      name: 'allure-auth',
      // Persist metadata only; the actual token is in sessionStorage
      partialize: (s) => ({
        isAuthenticated: s.isAuthenticated,
        roles: s.roles,
        username: s.username,
        expiresAt: s.expiresAt,
      }),
      // On hydration, restore the token from sessionStorage if still valid
      onRehydrateStorage: () => (state) => {
        if (!state) return
        const storedToken = sessionStorage.getItem('allure-token')
        const storedExpiry = sessionStorage.getItem('allure-token-expiry')
        if (
          storedToken &&
          storedExpiry &&
          parseInt(storedExpiry, 10) > Date.now()
        ) {
          setAccessToken(storedToken)
        } else {
          // Token expired or missing — clear persisted auth
          state.clearAuth()
        }
      },
    },
  ),
)
