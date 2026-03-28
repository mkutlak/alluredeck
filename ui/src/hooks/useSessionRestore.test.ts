import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useSessionRestore } from './useSessionRestore'
import { useAuthStore } from '@/store/auth'

vi.mock('@/api/auth', () => ({
  getSession: vi.fn(),
}))

import { getSession } from '@/api/auth'

const mockGetSession = vi.mocked(getSession)

describe('useSessionRestore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset store to default unauthenticated state
    useAuthStore.setState({
      isAuthenticated: false,
      roles: [],
      username: null,
      expiresAt: null,
      provider: null,
    })
  })

  it('calls getSession() and restores roles when authenticated but roles are empty', async () => {
    useAuthStore.setState({
      isAuthenticated: true,
      roles: [],
      username: 'alice',
      expiresAt: Date.now() + 3600 * 1000,
      provider: 'local',
    })

    mockGetSession.mockResolvedValue({
      data: { username: 'alice', roles: ['admin'], expires_in: 3600, provider: 'local' },
      metadata: { message: 'ok' },
    })

    const { result } = renderHook(() => useSessionRestore())

    await waitFor(() => {
      expect(result.current.isRestoring).toBe(false)
    })

    expect(mockGetSession).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().roles).toEqual(['admin'])
  })

  it('calls clearAuth() when getSession() returns a 401 error', async () => {
    useAuthStore.setState({
      isAuthenticated: true,
      roles: [],
      username: 'alice',
      expiresAt: Date.now() + 3600 * 1000,
      provider: 'local',
    })

    mockGetSession.mockRejectedValue({ response: { status: 401 } })

    const { result } = renderHook(() => useSessionRestore())

    await waitFor(() => {
      expect(result.current.isRestoring).toBe(false)
    })

    expect(mockGetSession).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().isAuthenticated).toBe(false)
    expect(useAuthStore.getState().roles).toEqual([])
  })

  it('does NOT call getSession() when roles are already populated', () => {
    useAuthStore.setState({
      isAuthenticated: true,
      roles: ['admin'],
      username: 'alice',
      expiresAt: Date.now() + 3600 * 1000,
      provider: 'local',
    })

    renderHook(() => useSessionRestore())

    expect(mockGetSession).not.toHaveBeenCalled()
  })

  it('does NOT call getSession() when isAuthenticated is false', () => {
    useAuthStore.setState({
      isAuthenticated: false,
      roles: [],
      username: null,
      expiresAt: null,
      provider: null,
    })

    renderHook(() => useSessionRestore())

    expect(mockGetSession).not.toHaveBeenCalled()
  })

  it('returns isRestoring: true while fetching, isRestoring: false when done', async () => {
    useAuthStore.setState({
      isAuthenticated: true,
      roles: [],
      username: 'alice',
      expiresAt: Date.now() + 3600 * 1000,
      provider: 'local',
    })

    let resolveSession!: (value: Awaited<ReturnType<typeof getSession>>) => void
    mockGetSession.mockReturnValue(
      new Promise<Awaited<ReturnType<typeof getSession>>>((resolve) => {
        resolveSession = resolve
      }),
    )

    const { result } = renderHook(() => useSessionRestore())

    // Should be restoring initially
    expect(result.current.isRestoring).toBe(true)

    // Resolve the promise
    resolveSession({
      data: { username: 'alice', roles: ['admin'], expires_in: 3600, provider: 'local' },
      metadata: { message: 'ok' },
    })

    await waitFor(() => {
      expect(result.current.isRestoring).toBe(false)
    })
  })

  it('does not double-fetch in React StrictMode (useRef guard)', async () => {
    useAuthStore.setState({
      isAuthenticated: true,
      roles: [],
      username: 'alice',
      expiresAt: Date.now() + 3600 * 1000,
      provider: 'local',
    })

    mockGetSession.mockResolvedValue({
      data: { username: 'alice', roles: ['admin'], expires_in: 3600, provider: 'local' },
      metadata: { message: 'ok' },
    })

    const { result } = renderHook(() => useSessionRestore())

    await waitFor(() => {
      expect(result.current.isRestoring).toBe(false)
    })

    // Should only be called once despite StrictMode double-invoke
    expect(mockGetSession).toHaveBeenCalledTimes(1)
  })
})
