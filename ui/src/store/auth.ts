import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'

export type Role = 'admin' | 'viewer'

export interface AuthState {
  isAuthenticated: boolean
  roles: Role[]
  username: string | null
  expiresAt: number | null

  setAuth: (roles: Role[], username: string, expiresIn: number) => void
  clearAuth: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
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
    }),
    {
      name: 'allure-auth',
      storage: createJSONStorage(() => sessionStorage),
      partialize: (s) => ({
        isAuthenticated: s.isAuthenticated,
        // roles intentionally excluded — re-derived from server; client state is UI-only
        username: s.username,
        expiresAt: s.expiresAt,
      }),
    },
  ),
)

export const selectIsAdmin = (s: AuthState) => s.roles.includes('admin')

export const selectIsSessionValid = (s: AuthState) =>
  s.isAuthenticated && (s.expiresAt === null || s.expiresAt > Date.now())
