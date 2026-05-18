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
  pinnedProjectIds: number[]
  recentProjectIds: number[]
  lastTabPerProject: Record<string, string>

  setProjectViewMode: (mode: ViewMode) => void
  setLastProjectId: (id: string | null) => void
  clearLastProjectId: () => void
  setReportsPerPage: (perPage: number) => void
  setReportsGroupBy: (groupBy: GroupByMode) => void
  setSelectedBranch: (branch: string | undefined) => void
  setTimezone: (tz: string | null) => void
  setTimeFormat: (fmt: '12h' | '24h' | null) => void
  setSyncedAt: (ts: string | null) => void
  pinProject: (id: number) => void
  unpinProject: (id: number) => void
  recordProjectVisit: (id: number) => void
  setLastTabForProject: (projectId: string, tab: string) => void
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
      pinnedProjectIds: [],
      recentProjectIds: [],
      lastTabPerProject: {},

      setProjectViewMode: (mode) => set({ projectViewMode: mode }),
      setLastProjectId: (id) => set({ lastProjectId: id }),
      clearLastProjectId: () => set({ lastProjectId: null }),
      setReportsPerPage: (perPage) => set({ reportsPerPage: perPage }),
      setReportsGroupBy: (groupBy) => set({ reportsGroupBy: groupBy }),
      setSelectedBranch: (branch) => set({ selectedBranch: branch }),
      setTimezone: (tz) => set({ timezone: tz }),
      setTimeFormat: (fmt) => set({ timeFormat: fmt }),
      setSyncedAt: (ts) => set({ _syncedAt: ts }),
      pinProject: (id) =>
        set((s) => ({
          pinnedProjectIds: s.pinnedProjectIds.includes(id)
            ? s.pinnedProjectIds
            : [...s.pinnedProjectIds, id],
        })),
      unpinProject: (id) =>
        set((s) => ({ pinnedProjectIds: s.pinnedProjectIds.filter((p) => p !== id) })),
      recordProjectVisit: (id) =>
        set((s) => {
          const deduped = s.recentProjectIds.filter((p) => p !== id)
          return { recentProjectIds: [id, ...deduped].slice(0, 5) }
        }),
      setLastTabForProject: (projectId, tab) =>
        set((s) => ({ lastTabPerProject: { ...s.lastTabPerProject, [projectId]: tab } })),
    }),
    { name: 'allure-ui' },
  ),
)
