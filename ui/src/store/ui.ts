import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type ViewMode = 'grid' | 'table'
type Theme = 'light' | 'dark' | 'system'

interface UIState {
  theme: Theme
  projectViewMode: ViewMode

  setTheme: (theme: Theme) => void
  setProjectViewMode: (mode: ViewMode) => void
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      theme: 'system',
      projectViewMode: 'grid',

      setTheme: (theme) => set({ theme }),
      setProjectViewMode: (mode) => set({ projectViewMode: mode }),
    }),
    { name: 'allure-ui' },
  ),
)
