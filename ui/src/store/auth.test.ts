import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useAuthStore, selectIsAdmin, selectIsEditor } from './auth'
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

  describe('selectIsEditor', () => {
    it('returns true for admin role', () => {
      useAuthStore.getState().setAuth(['admin'], 'alice', 3600)
      expect(selectIsEditor(useAuthStore.getState())).toBe(true)
    })

    it('returns true for editor role', () => {
      useAuthStore.getState().setAuth(['editor'], 'bob', 3600)
      expect(selectIsEditor(useAuthStore.getState())).toBe(true)
    })

    it('returns false for viewer role', () => {
      useAuthStore.getState().setAuth(['viewer'], 'carol', 3600)
      expect(selectIsEditor(useAuthStore.getState())).toBe(false)
    })

    it('returns false when not authenticated', () => {
      expect(selectIsEditor(useAuthStore.getState())).toBe(false)
    })
  })
})
