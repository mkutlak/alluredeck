import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type Role = 'admin' | 'viewer'

export interface AuthState {
  isAuthenticated: boolean
  roles: Role[]
  username: string | null
  expiresAt: number | null

  setAuth: (roles: Role[], username: string, expiresIn: number) => void
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

      setAuth(roles, username, expiresIn) {
        set({
          isAuthenticated: true,
          roles,
          username,
          expiresAt: Date.now() + expiresIn * 1000,
        })
      },

      clearAuth() {
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
      partialize: (s) => ({
        isAuthenticated: s.isAuthenticated,
        roles: s.roles,
        username: s.username,
        expiresAt: s.expiresAt,
      }),
    },
  ),
)
