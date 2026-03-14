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

  setProjectViewMode: (mode: ViewMode) => void
  setLastProjectId: (id: string | null) => void
  clearLastProjectId: () => void
  setReportsPerPage: (perPage: number) => void
  setReportsGroupBy: (groupBy: GroupByMode) => void
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      projectViewMode: 'grid',
      lastProjectId: null,
      reportsPerPage: 20,
      reportsGroupBy: 'none',

      setProjectViewMode: (mode) => set({ projectViewMode: mode }),
      setLastProjectId: (id) => set({ lastProjectId: id }),
      clearLastProjectId: () => set({ lastProjectId: null }),
      setReportsPerPage: (perPage) => set({ reportsPerPage: perPage }),
      setReportsGroupBy: (groupBy) => set({ reportsGroupBy: groupBy }),
    }),
    { name: 'allure-ui' },
  ),
)
