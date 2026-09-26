import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { usePreferencesSync } from './usePreferencesSync'
import { useUIStore, type UIState } from '@/store/ui'
import { useAuthStore } from '@/store/auth'

vi.mock('@/api/preferences', () => ({
  fetchPreferences: vi.fn(),
  updatePreferences: vi.fn(),
}))

import { fetchPreferences, updatePreferences } from '@/api/preferences'

const mockFetch = vi.mocked(fetchPreferences)
const mockUpdate = vi.mocked(updatePreferences)

function serverReturns(preferences: Record<string, unknown>, updated_at: string) {
  mockFetch.mockResolvedValue({ data: { preferences, updated_at }, metadata: { message: 'ok' } })
}

describe('usePreferencesSync', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    useUIStore.setState({ _syncedAt: null })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does nothing when user is not authenticated', () => {
    useAuthStore.setState({ isAuthenticated: false })
    renderHook(() => usePreferencesSync())
    expect(mockFetch).not.toHaveBeenCalled()
  })

  it.each<[string, Partial<UIState>, Record<string, unknown>, string, Partial<UIState>]>([
    [
      'seeds preferences when the server is newer',
      { projectViewMode: 'grid' },
      { projectViewMode: 'table' },
      '2026-04-06T12:00:00Z',
      { projectViewMode: 'table', _syncedAt: '2026-04-06T12:00:00Z' },
    ],
    [
      'skips seeding when the server has no preferences',
      { projectViewMode: 'grid' },
      {},
      '',
      { projectViewMode: 'grid', _syncedAt: null },
    ],
    [
      'coerces a non-array runsFeedGroupIds from the server into an empty array',
      { runsFeedGroupIds: [1, 2] },
      { runsFeedGroupIds: 'not-an-array' },
      '2026-04-06T12:00:00Z',
      { runsFeedGroupIds: [] },
    ],
  ])('on mount %s', async (_, local, preferences, updatedAt, expected) => {
    useAuthStore.setState({ isAuthenticated: true })
    useUIStore.setState(local)
    serverReturns(preferences, updatedAt)

    renderHook(() => usePreferencesSync())
    await vi.waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1))

    expect(useUIStore.getState()).toMatchObject(expected)
  })

  it('debounces state changes and flushes to server after 3s', async () => {
    useAuthStore.setState({ isAuthenticated: true })
    serverReturns({}, '')
    mockUpdate.mockResolvedValue({
      data: { preferences: {}, updated_at: '2026-04-06T12:01:00Z' },
      metadata: { message: 'ok' },
    })

    renderHook(() => usePreferencesSync())
    await vi.waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1))

    act(() => {
      useUIStore.setState({ projectViewMode: 'table' })
    })
    expect(mockUpdate).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(3500)
    })

    expect(mockUpdate).toHaveBeenCalledTimes(1)
    expect(mockUpdate).toHaveBeenCalledWith(expect.objectContaining({ projectViewMode: 'table' }))
  })
})
