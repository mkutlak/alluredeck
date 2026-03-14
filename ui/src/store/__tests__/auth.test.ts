import { describe, it, expect, beforeEach } from 'vitest'
import { useAuthStore, selectIsAdmin, selectIsEditor, selectIsSessionValid } from '../auth'
import type { AuthState } from '../auth'

function getState(): AuthState {
  return useAuthStore.getState()
}

describe('useAuthStore', () => {
  beforeEach(() => {
    useAuthStore.getState().clearAuth()
  })

  describe('setAuth', () => {
    it('sets authentication state with local provider by default', () => {
      getState().setAuth(['admin'], 'admin-user', 3600)

      const s = getState()
      expect(s.isAuthenticated).toBe(true)
      expect(s.roles).toEqual(['admin'])
      expect(s.username).toBe('admin-user')
      expect(s.provider).toBe('local')
      expect(s.expiresAt).toBeGreaterThan(Date.now())
    })

    it('sets authentication state with oidc provider', () => {
      getState().setAuth(['editor'], 'sso-user', 3600, 'oidc')

      const s = getState()
      expect(s.isAuthenticated).toBe(true)
      expect(s.roles).toEqual(['editor'])
      expect(s.username).toBe('sso-user')
      expect(s.provider).toBe('oidc')
    })

    it('sets provider to local when explicitly passed', () => {
      getState().setAuth(['viewer'], 'local-user', 3600, 'local')

      expect(getState().provider).toBe('local')
    })
  })

  describe('clearAuth', () => {
    it('resets all auth state including provider', () => {
      getState().setAuth(['admin'], 'admin-user', 3600, 'oidc')
      getState().clearAuth()

      const s = getState()
      expect(s.isAuthenticated).toBe(false)
      expect(s.roles).toEqual([])
      expect(s.username).toBeNull()
      expect(s.provider).toBeNull()
      expect(s.expiresAt).toBeNull()
    })
  })

  describe('selectIsAdmin', () => {
    it('returns true for admin role', () => {
      getState().setAuth(['admin'], 'a', 3600)
      expect(selectIsAdmin(getState())).toBe(true)
    })

    it('returns false for editor role', () => {
      getState().setAuth(['editor'], 'e', 3600)
      expect(selectIsAdmin(getState())).toBe(false)
    })

    it('returns false for viewer role', () => {
      getState().setAuth(['viewer'], 'v', 3600)
      expect(selectIsAdmin(getState())).toBe(false)
    })
  })

  describe('selectIsEditor', () => {
    it('returns true for admin role', () => {
      getState().setAuth(['admin'], 'a', 3600)
      expect(selectIsEditor(getState())).toBe(true)
    })

    it('returns true for editor role', () => {
      getState().setAuth(['editor'], 'e', 3600)
      expect(selectIsEditor(getState())).toBe(true)
    })

    it('returns false for viewer role', () => {
      getState().setAuth(['viewer'], 'v', 3600)
      expect(selectIsEditor(getState())).toBe(false)
    })
  })

  describe('selectIsSessionValid', () => {
    it('returns true when authenticated and not expired', () => {
      getState().setAuth(['viewer'], 'v', 3600)
      expect(selectIsSessionValid(getState())).toBe(true)
    })

    it('returns false when not authenticated', () => {
      expect(selectIsSessionValid(getState())).toBe(false)
    })
  })
})
