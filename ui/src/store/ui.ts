import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type ViewMode = 'grid' | 'table'

interface UIState {
  projectViewMode: ViewMode

  setProjectViewMode: (mode: ViewMode) => void
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      projectViewMode: 'grid',

      setProjectViewMode: (mode) => set({ projectViewMode: mode }),
    }),
    { name: 'allure-ui' },
  ),
)
