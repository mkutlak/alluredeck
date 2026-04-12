import { render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthGuard } from './AuthGuard'
import { attemptRefresh } from '@/api/client'
import { useAuthStore } from '@/store/auth'

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  attemptRefresh: vi.fn(),
}))

vi.mock('react-router', () => ({
  Navigate: () => null,
  useLocation: () => ({ pathname: '/' }),
}))

// Token TTL large enough that the proactive refresh timer fires after we
// advance fake timers, rather than firing synchronously on first render.
// (AuthGuard refreshes REFRESH_MARGIN_MS=60s before real expiry, so we need
// remaining > 60s to NOT refresh immediately.)
const TTL_AHEAD_MS = 5 * 60 * 1000 // 5 minutes
const SCHEDULED_FIRE_MS = TTL_AHEAD_MS - 60 * 1000 // margin is 60s

function setupStore(overrides: { expiresAt: number | null; isAuthenticated?: boolean }) {
  const mockClearAuth = vi.fn()
  vi.mocked(useAuthStore).mockImplementation((selector) =>
    selector({
      isAuthenticated: overrides.isAuthenticated ?? true,
      expiresAt: overrides.expiresAt,
      clearAuth: mockClearAuth,
      // Non-empty roles so useSessionRestore.needsRestore === false and the
      // hook doesn't fire an unmocked /auth/session fetch that would fail
      // in jsdom and call clearAuth in its .catch branch, polluting asserts.
      roles: ['admin'],
      username: 'admin',
      provider: 'local',
      setAuth: vi.fn(),
    }),
  )
  return mockClearAuth
}

// Flush microtasks so async .then chains inside the effect run under fake timers.
async function flushMicrotasks() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('AuthGuard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.mocked(attemptRefresh).mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('attempts refresh before clearing auth when the proactive timer fires', async () => {
    vi.mocked(attemptRefresh).mockResolvedValue(false)
    const expiresAt = Date.now() + TTL_AHEAD_MS
    const mockClearAuth = setupStore({ expiresAt })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    // Timer hasn't fired yet — neither refresh nor clearAuth should have run.
    expect(attemptRefresh).not.toHaveBeenCalled()
    expect(mockClearAuth).not.toHaveBeenCalled()

    vi.advanceTimersByTime(SCHEDULED_FIRE_MS + 1)
    await flushMicrotasks()

    expect(attemptRefresh).toHaveBeenCalledTimes(1)
    expect(mockClearAuth).toHaveBeenCalledTimes(1)
  })

  it('does not call clearAuth when refresh succeeds', async () => {
    vi.mocked(attemptRefresh).mockResolvedValue(true)
    const expiresAt = Date.now() + TTL_AHEAD_MS
    const mockClearAuth = setupStore({ expiresAt })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    vi.advanceTimersByTime(SCHEDULED_FIRE_MS + 1)
    await flushMicrotasks()

    expect(attemptRefresh).toHaveBeenCalledTimes(1)
    expect(mockClearAuth).not.toHaveBeenCalled()
  })

  it('does not call refresh or clearAuth after unmount', async () => {
    vi.mocked(attemptRefresh).mockResolvedValue(false)
    const expiresAt = Date.now() + TTL_AHEAD_MS
    const mockClearAuth = setupStore({ expiresAt })

    const { unmount } = render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    unmount()
    vi.advanceTimersByTime(SCHEDULED_FIRE_MS + 1)
    await flushMicrotasks()

    expect(attemptRefresh).not.toHaveBeenCalled()
    expect(mockClearAuth).not.toHaveBeenCalled()
  })

  it('does nothing when expiresAt is null', async () => {
    vi.mocked(attemptRefresh).mockResolvedValue(false)
    const mockClearAuth = setupStore({ expiresAt: null })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    vi.advanceTimersByTime(60 * 60 * 1000)
    await flushMicrotasks()

    expect(attemptRefresh).not.toHaveBeenCalled()
    expect(mockClearAuth).not.toHaveBeenCalled()
  })

  it('attempts refresh immediately when already expired and clears auth on failure', async () => {
    vi.mocked(attemptRefresh).mockResolvedValue(false)
    const expiresAt = Date.now() - 1000
    const mockClearAuth = setupStore({ expiresAt })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    await flushMicrotasks()

    expect(attemptRefresh).toHaveBeenCalledTimes(1)
    expect(mockClearAuth).toHaveBeenCalledTimes(1)
  })

  it('attempts refresh immediately when already expired and skips clearAuth on success', async () => {
    vi.mocked(attemptRefresh).mockResolvedValue(true)
    const expiresAt = Date.now() - 1000
    const mockClearAuth = setupStore({ expiresAt })

    render(
      <AuthGuard>
        <div>protected</div>
      </AuthGuard>,
    )

    await flushMicrotasks()

    expect(attemptRefresh).toHaveBeenCalledTimes(1)
    expect(mockClearAuth).not.toHaveBeenCalled()
  })
})
