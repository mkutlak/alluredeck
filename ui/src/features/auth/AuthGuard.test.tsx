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

function renderGuard(expiresAt: number | null) {
  const clearAuth = vi.fn()
  vi.mocked(useAuthStore).mockImplementation((selector) =>
    selector({
      isAuthenticated: true,
      expiresAt,
      clearAuth,
      // Non-empty roles so useSessionRestore.needsRestore === false and the
      // hook doesn't fire an unmocked /auth/session fetch that would fail
      // in jsdom and call clearAuth in its .catch branch, polluting asserts.
      roles: ['admin'],
      username: 'admin',
      provider: 'local',
      setAuth: vi.fn(),
    }),
  )
  const view = render(
    <AuthGuard>
      <div>protected</div>
    </AuthGuard>,
  )
  return { clearAuth, unmount: view.unmount }
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

  it.each([
    { when: 'the proactive timer fires', ttl: TTL_AHEAD_MS, refreshed: false },
    { when: 'the proactive timer fires', ttl: TTL_AHEAD_MS, refreshed: true },
    { when: 'already expired', ttl: -1000, refreshed: false },
    { when: 'already expired', ttl: -1000, refreshed: true },
  ])(
    'attempts refresh when $when, clearing auth only if refresh fails (ok=$refreshed)',
    async ({ ttl, refreshed }) => {
      vi.mocked(attemptRefresh).mockResolvedValue(refreshed)
      const { clearAuth } = renderGuard(Date.now() + ttl)

      if (ttl > 60 * 1000) {
        // Timer hasn't fired yet — neither refresh nor clearAuth should have run.
        await flushMicrotasks()
        expect(attemptRefresh).not.toHaveBeenCalled()
        vi.advanceTimersByTime(SCHEDULED_FIRE_MS + 1)
      }
      await flushMicrotasks()

      expect(attemptRefresh).toHaveBeenCalledTimes(1)
      expect(clearAuth).toHaveBeenCalledTimes(refreshed ? 0 : 1)
    },
  )

  it.each([
    { name: 'expiresAt is null', ttl: null, unmount: false },
    { name: 'after unmount', ttl: TTL_AHEAD_MS, unmount: true },
  ])('does not refresh or clear auth when $name', async ({ ttl, unmount }) => {
    vi.mocked(attemptRefresh).mockResolvedValue(false)
    const view = renderGuard(ttl === null ? null : Date.now() + ttl)

    if (unmount) view.unmount()
    vi.advanceTimersByTime(60 * 60 * 1000)
    await flushMicrotasks()

    expect(attemptRefresh).not.toHaveBeenCalled()
    expect(view.clearAuth).not.toHaveBeenCalled()
  })
})
