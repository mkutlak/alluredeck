import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useSessionRestore } from './useSessionRestore'
import { useAuthStore, type AuthState } from '@/store/auth'

vi.mock('@/api/auth', () => ({
  getSession: vi.fn(),
}))

import { getSession } from '@/api/auth'

const mockGetSession = vi.mocked(getSession)
// Post-refresh state: roles are not persisted, so they come back empty (fix 814b279).
const restored: Partial<AuthState> = {
  isAuthenticated: true,
  roles: [],
  username: 'alice',
  expiresAt: Date.now() + 3600 * 1000,
  provider: 'local',
}
const session = {
  data: { username: 'alice', roles: ['admin'], expires_in: 3600, provider: 'local' as const },
  metadata: { message: 'ok' },
}

describe('useSessionRestore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().clearAuth()
  })

  it('re-fetches the session once (even under StrictMode) and restores the roles', async () => {
    useAuthStore.setState(restored)
    let resolveSession!: (value: typeof session) => void
    mockGetSession.mockReturnValue(new Promise((resolve) => (resolveSession = resolve)))

    const { result } = renderHook(() => useSessionRestore(), { reactStrictMode: true })
    expect(result.current.isRestoring).toBe(true)

    resolveSession(session)
    await waitFor(() => expect(result.current.isRestoring).toBe(false))

    expect(mockGetSession).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().roles).toEqual(['admin'])
  })

  it('calls clearAuth() when getSession() returns a 401 error', async () => {
    useAuthStore.setState(restored)
    mockGetSession.mockRejectedValue({ response: { status: 401 } })

    const { result } = renderHook(() => useSessionRestore())
    await waitFor(() => expect(result.current.isRestoring).toBe(false))

    expect(mockGetSession).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState()).toMatchObject({ isAuthenticated: false, roles: [] })
  })

  it.each<[string, Partial<AuthState>]>([
    ['roles are already populated', { ...restored, roles: ['admin'] }],
    ['isAuthenticated is false', {}],
  ])('does NOT call getSession() when %s', (_, state) => {
    useAuthStore.setState(state)
    renderHook(() => useSessionRestore())
    expect(mockGetSession).not.toHaveBeenCalled()
  })
})
