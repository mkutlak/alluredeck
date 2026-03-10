import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type ViewMode = 'grid' | 'table'

export interface UIState {
  projectViewMode: ViewMode
  lastProjectId: string | null

  setProjectViewMode: (mode: ViewMode) => void
  setLastProjectId: (id: string | null) => void
  clearLastProjectId: () => void
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      projectViewMode: 'grid',
      lastProjectId: null,

      setProjectViewMode: (mode) => set({ projectViewMode: mode }),
      setLastProjectId: (id) => set({ lastProjectId: id }),
      clearLastProjectId: () => set({ lastProjectId: null }),
    }),
    { name: 'allure-ui' },
  ),
)
