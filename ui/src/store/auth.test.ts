import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useAuthStore, selectIsAdmin, selectIsSessionValid } from './auth'
import { mockApiClient } from '@/test/mocks/api-client'

// Mock the API client module — no more setAccessToken
mockApiClient()

describe('useAuthStore', () => {
  beforeEach(() => {
    // Reset store state
    useAuthStore.setState({
      isAuthenticated: false,
      roles: [],
      username: null,
      expiresAt: null,
    })
    vi.clearAllMocks()
  })

  describe('setAuth', () => {
    it('sets authenticated state', () => {
      useAuthStore.getState().setAuth(['admin'], 'alice', 3600)
      const state = useAuthStore.getState()
      expect(state.isAuthenticated).toBe(true)
      expect(state.username).toBe('alice')
      expect(state.roles).toEqual(['admin'])
    })

    it('sets expiresAt in the future', () => {
      const before = Date.now()
      useAuthStore.getState().setAuth(['admin'], 'alice', 3600)
      const { expiresAt } = useAuthStore.getState()
      expect(expiresAt).toBeGreaterThan(before + 3_599_000)
    })
  })

  describe('clearAuth', () => {
    it('resets all auth state', () => {
      useAuthStore.getState().setAuth(['admin'], 'alice', 3600)
      useAuthStore.getState().clearAuth()
      const state = useAuthStore.getState()
      expect(state.isAuthenticated).toBe(false)
      expect(state.username).toBeNull()
      expect(state.roles).toEqual([])
      expect(state.expiresAt).toBeNull()
    })
  })

  describe('isAdmin', () => {
    it('returns true for admin role', () => {
      useAuthStore.getState().setAuth(['admin'], 'alice', 3600)
      expect(selectIsAdmin(useAuthStore.getState())).toBe(true)
    })

    it('returns false for viewer role', () => {
      useAuthStore.getState().setAuth(['viewer'], 'bob', 3600)
      expect(selectIsAdmin(useAuthStore.getState())).toBe(false)
    })

    it('returns false when not authenticated', () => {
      expect(selectIsAdmin(useAuthStore.getState())).toBe(false)
    })
  })

  describe('isSessionValid', () => {
    it('returns false when not authenticated', () => {
      expect(selectIsSessionValid(useAuthStore.getState())).toBe(false)
    })

    it('returns true when authenticated and token not expired', () => {
      useAuthStore.getState().setAuth(['admin'], 'alice', 3600)
      expect(selectIsSessionValid(useAuthStore.getState())).toBe(true)
    })

    it('returns false when token is expired', () => {
      expect(
        selectIsSessionValid({
          ...useAuthStore.getState(),
          isAuthenticated: true,
          roles: ['admin'],
          username: 'alice',
          expiresAt: Date.now() - 1000, // already expired
        }),
      ).toBe(false)
    })
  })
})
