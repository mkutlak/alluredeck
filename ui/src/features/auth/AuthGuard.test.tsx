import { render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthGuard } from './AuthGuard'
import { useAuthStore } from '@/store/auth'

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
}))

vi.mock('react-router', () => ({
  Navigate: () => null,
  useLocation: () => ({ pathname: '/' }),
}))

function setupStore(overrides: { expiresAt: number | null; isAuthenticated?: boolean }) {
  const mockClearAuth = vi.fn()
  vi.mocked(useAuthStore).mockImplementation((selector) =>
    selector({
        isAuthenticated: overrides.isAuthenticated ?? true,
        expiresAt: overrides.expiresAt,
        clearAuth: mockClearAuth,
        roles: [],
        username: null,
        setAuth: vi.fn(),
      }),
  )
  return mockClearAuth
}

describe('AuthGuard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('calls clearAuth after timeout elapses', () => {
    const expiresAt = Date.now() + 5000
    const mockClearAuth = setupStore({ expiresAt })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    expect(mockClearAuth).not.toHaveBeenCalled()
    vi.advanceTimersByTime(5001)
    expect(mockClearAuth).toHaveBeenCalledTimes(1)
  })

  it('clears timer on unmount without calling clearAuth', () => {
    const expiresAt = Date.now() + 5000
    const mockClearAuth = setupStore({ expiresAt })

    const { unmount } = render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    unmount()
    vi.advanceTimersByTime(5001)
    expect(mockClearAuth).not.toHaveBeenCalled()
  })

  it('does not set a timer when expiresAt is null', () => {
    const mockClearAuth = setupStore({ expiresAt: null })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    vi.advanceTimersByTime(60 * 60 * 1000)
    expect(mockClearAuth).not.toHaveBeenCalled()
  })

  it('calls clearAuth immediately when session is already expired', () => {
    const expiresAt = Date.now() - 1000
    const mockClearAuth = setupStore({ expiresAt })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    expect(mockClearAuth).toHaveBeenCalledTimes(1)
  })
})
