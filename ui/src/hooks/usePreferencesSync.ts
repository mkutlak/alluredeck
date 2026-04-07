import { useEffect, useRef } from 'react'
import { useAuthStore } from '@/store/auth'
import { useUIStore, type UIState } from '@/store/ui'
import { fetchPreferences, updatePreferences } from '@/api/preferences'

/** Keys from UIState that are persisted to the server. */
const SYNC_KEYS: readonly (keyof UIState)[] = [
  'projectViewMode',
  'lastProjectId',
  'reportsPerPage',
  'reportsGroupBy',
  'selectedBranch',
] as const

function pickSyncState(state: UIState): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const key of SYNC_KEYS) {
    result[key] = state[key]
  }
  return result
}

export function usePreferencesSync(): void {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)

  // Seed from server on mount
  useEffect(() => {
    if (!isAuthenticated) return

    let cancelled = false

    async function seed() {
      try {
        const res = await fetchPreferences()
        if (cancelled) return

        const { preferences, updated_at } = res.data
        if (!updated_at) return // new user, no server state

        const localSyncedAt = useUIStore.getState()._syncedAt
        const serverNewer = !localSyncedAt || new Date(updated_at) > new Date(localSyncedAt)

        if (serverNewer && Object.keys(preferences).length > 0) {
          useUIStore.setState({ ...preferences, _syncedAt: updated_at })
        }
      } catch {
        // Best-effort — localStorage is the source of truth
      }
    }

    void seed()
    return () => {
      cancelled = true
    }
  }, [isAuthenticated])

  // Subscribe to state changes and debounce writes
  useEffect(() => {
    if (!isAuthenticated) return

    mountedRef.current = true

    const unsub = useUIStore.subscribe(() => {
      if (timerRef.current) clearTimeout(timerRef.current)

      timerRef.current = setTimeout(() => {
        if (!mountedRef.current) return

        const state = useUIStore.getState()
        const payload = pickSyncState(state)

        updatePreferences(payload)
          .then((res) => {
            if (mountedRef.current && res.data.updated_at) {
              useUIStore.setState({ _syncedAt: res.data.updated_at })
            }
          })
          .catch(() => {
            // Best-effort — will retry on next state change
          })
      }, 3000)
    })

    return () => {
      mountedRef.current = false
      if (timerRef.current) clearTimeout(timerRef.current)
      unsub()
    }
  }, [isAuthenticated])
}
