import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type ViewMode = 'grid' | 'table'
export type GroupByMode = 'none' | 'commit' | 'branch'
export const PER_PAGE_OPTIONS = [10, 20, 50, 100] as const

export interface UIState {
  projectViewMode: ViewMode
  lastProjectId: string | null
  reportsPerPage: number
  reportsGroupBy: GroupByMode
  selectedBranch: string | undefined
  timezone: string | null
  timeFormat: '12h' | '24h' | null
  _syncedAt: string | null

  setProjectViewMode: (mode: ViewMode) => void
  setLastProjectId: (id: string | null) => void
  clearLastProjectId: () => void
  setReportsPerPage: (perPage: number) => void
  setReportsGroupBy: (groupBy: GroupByMode) => void
  setSelectedBranch: (branch: string | undefined) => void
  setTimezone: (tz: string | null) => void
  setTimeFormat: (fmt: '12h' | '24h' | null) => void
  setSyncedAt: (ts: string | null) => void
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      projectViewMode: 'grid',
      lastProjectId: null,
      reportsPerPage: 20,
      reportsGroupBy: 'none',
      selectedBranch: undefined,
      timezone: null,
      timeFormat: null,
      _syncedAt: null,

      setProjectViewMode: (mode) => set({ projectViewMode: mode }),
      setLastProjectId: (id) => set({ lastProjectId: id }),
      clearLastProjectId: () => set({ lastProjectId: null }),
      setReportsPerPage: (perPage) => set({ reportsPerPage: perPage }),
      setReportsGroupBy: (groupBy) => set({ reportsGroupBy: groupBy }),
      setSelectedBranch: (branch) => set({ selectedBranch: branch }),
      setTimezone: (tz) => set({ timezone: tz }),
      setTimeFormat: (fmt) => set({ timeFormat: fmt }),
      setSyncedAt: (ts) => set({ _syncedAt: ts }),
    }),
    { name: 'allure-ui' },
  ),
)
