import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useAuthStore } from './auth'
import * as clientModule from '@/api/client'

// Mock the API client module
vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: {},
  extractErrorMessage: vi.fn(),
}))

// Mock sessionStorage
const sessionStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = value },
    removeItem: (key: string) => { delete store[key] },
    clear: () => { store = {} },
  }
})()
Object.defineProperty(window, 'sessionStorage', { value: sessionStorageMock })

describe('useAuthStore', () => {
  beforeEach(() => {
    // Reset store state
    useAuthStore.setState({
      isAuthenticated: false,
      roles: [],
      username: null,
      expiresAt: null,
    })
    sessionStorageMock.clear()
    vi.clearAllMocks()
  })

  describe('setAuth', () => {
    it('sets authenticated state', () => {
      useAuthStore.getState().setAuth('token123', ['admin'], 'alice', 3600)
      const state = useAuthStore.getState()
      expect(state.isAuthenticated).toBe(true)
      expect(state.username).toBe('alice')
      expect(state.roles).toEqual(['admin'])
    })

    it('calls setAccessToken with the token', () => {
      useAuthStore.getState().setAuth('tok', ['viewer'], 'bob', 1800)
      expect(clientModule.setAccessToken).toHaveBeenCalledWith('tok')
    })

    it('stores token in sessionStorage', () => {
      useAuthStore.getState().setAuth('tok', ['admin'], 'alice', 3600)
      expect(sessionStorageMock.getItem('allure-token')).toBe('tok')
    })

    it('sets expiresAt in the future', () => {
      const before = Date.now()
      useAuthStore.getState().setAuth('tok', ['admin'], 'alice', 3600)
      const { expiresAt } = useAuthStore.getState()
      expect(expiresAt).toBeGreaterThan(before + 3_599_000)
    })
  })

  describe('clearAuth', () => {
    it('resets all auth state', () => {
      useAuthStore.getState().setAuth('tok', ['admin'], 'alice', 3600)
      useAuthStore.getState().clearAuth()
      const state = useAuthStore.getState()
      expect(state.isAuthenticated).toBe(false)
      expect(state.username).toBeNull()
      expect(state.roles).toEqual([])
      expect(state.expiresAt).toBeNull()
    })

    it('clears sessionStorage', () => {
      useAuthStore.getState().setAuth('tok', ['admin'], 'alice', 3600)
      useAuthStore.getState().clearAuth()
      expect(sessionStorageMock.getItem('allure-token')).toBeNull()
    })

    it('calls setAccessToken(null)', () => {
      useAuthStore.getState().clearAuth()
      expect(clientModule.setAccessToken).toHaveBeenCalledWith(null)
    })
  })

  describe('isAdmin', () => {
    it('returns true for admin role', () => {
      useAuthStore.getState().setAuth('tok', ['admin'], 'alice', 3600)
      expect(useAuthStore.getState().isAdmin()).toBe(true)
    })

    it('returns false for viewer role', () => {
      useAuthStore.getState().setAuth('tok', ['viewer'], 'bob', 3600)
      expect(useAuthStore.getState().isAdmin()).toBe(false)
    })

    it('returns false when not authenticated', () => {
      expect(useAuthStore.getState().isAdmin()).toBe(false)
    })
  })

  describe('isSessionValid', () => {
    it('returns false when not authenticated', () => {
      expect(useAuthStore.getState().isSessionValid()).toBe(false)
    })

    it('returns true when authenticated and token not expired', () => {
      useAuthStore.getState().setAuth('tok', ['admin'], 'alice', 3600)
      expect(useAuthStore.getState().isSessionValid()).toBe(true)
    })

    it('returns false when token is expired', () => {
      useAuthStore.setState({
        isAuthenticated: true,
        roles: ['admin'],
        username: 'alice',
        expiresAt: Date.now() - 1000, // already expired
      })
      expect(useAuthStore.getState().isSessionValid()).toBe(false)
    })
  })
})
