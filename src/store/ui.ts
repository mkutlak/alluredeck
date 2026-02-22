import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type ViewMode = 'grid' | 'table'
type Theme = 'light' | 'dark' | 'system'

interface UIState {
  sidebarCollapsed: boolean
  theme: Theme
  projectViewMode: ViewMode

  toggleSidebar: () => void
  setSidebarCollapsed: (collapsed: boolean) => void
  setTheme: (theme: Theme) => void
  setProjectViewMode: (mode: ViewMode) => void
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      theme: 'system',
      projectViewMode: 'grid',

      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
      setTheme: (theme) => set({ theme }),
      setProjectViewMode: (mode) => set({ projectViewMode: mode }),
    }),
    { name: 'allure-ui' },
  ),
)
