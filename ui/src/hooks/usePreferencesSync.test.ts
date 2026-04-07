import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { usePreferencesSync } from './usePreferencesSync'
import { useUIStore } from '@/store/ui'
import { useAuthStore } from '@/store/auth'

vi.mock('@/api/preferences', () => ({
  fetchPreferences: vi.fn(),
  updatePreferences: vi.fn(),
}))

import { fetchPreferences, updatePreferences } from '@/api/preferences'

const mockFetch = vi.mocked(fetchPreferences)
const mockUpdate = vi.mocked(updatePreferences)

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

  it('fetches and seeds preferences on mount when server is newer', async () => {
    useAuthStore.setState({ isAuthenticated: true })
    useUIStore.setState({ _syncedAt: null, projectViewMode: 'grid' })

    mockFetch.mockResolvedValue({
      data: {
        preferences: { projectViewMode: 'table' },
        updated_at: '2026-04-06T12:00:00Z',
      },
      metadata: { message: 'ok' },
    })

    renderHook(() => usePreferencesSync())
    await vi.waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1))

    expect(useUIStore.getState().projectViewMode).toBe('table')
    expect(useUIStore.getState()._syncedAt).toBe('2026-04-06T12:00:00Z')
  })

  it('skips seeding when server has no preferences', async () => {
    useAuthStore.setState({ isAuthenticated: true })
    useUIStore.setState({ projectViewMode: 'grid' })

    mockFetch.mockResolvedValue({
      data: { preferences: {}, updated_at: '' },
      metadata: { message: 'ok' },
    })

    renderHook(() => usePreferencesSync())
    await vi.waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1))

    expect(useUIStore.getState().projectViewMode).toBe('grid')
  })

  it('debounces state changes and flushes to server after 3s', async () => {
    useAuthStore.setState({ isAuthenticated: true })
    mockFetch.mockResolvedValue({
      data: { preferences: {}, updated_at: '' },
      metadata: { message: 'ok' },
    })
    mockUpdate.mockResolvedValue({
      data: { preferences: {}, updated_at: '2026-04-06T12:01:00Z' },
      metadata: { message: 'ok' },
    })

    renderHook(() => usePreferencesSync())
    await vi.waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1))

    // Trigger a state change
    act(() => useUIStore.setState({ projectViewMode: 'table' }))

    // Not flushed yet
    expect(mockUpdate).not.toHaveBeenCalled()

    // Advance past debounce
    await act(async () => {
      vi.advanceTimersByTime(3500)
    })

    expect(mockUpdate).toHaveBeenCalledTimes(1)
    expect(mockUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ projectViewMode: 'table' }),
    )
  })
})
