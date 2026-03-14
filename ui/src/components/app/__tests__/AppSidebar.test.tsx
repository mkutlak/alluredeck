import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { SidebarProvider } from '@/components/ui/sidebar'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useAuthStore } from '@/store/auth'
import type { AuthState, Role } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import type { UIState } from '@/store/ui'
import { getProjects } from '@/api/projects'
import { AppSidebar } from '../AppSidebar'

vi.mock('@/api/projects', () => ({
  getProjects: vi.fn().mockResolvedValue({
    data: [{ project_id: 'project-alpha' }, { project_id: 'my-project' }],
    metadata: { message: 'ok' },
    pagination: { total: 2, page: 1, per_page: 20, total_pages: 1 },
  }),
}))

import { mockApiClient } from '@/test/mocks/api-client'
mockApiClient()

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
}))

vi.mock('@/store/ui', () => ({
  useUIStore: vi.fn(),
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

function makeUIState(overrides: Partial<UIState> = {}): UIState {
  return {
    projectViewMode: 'grid',
    lastProjectId: null,
    reportsPerPage: 20,
    reportsGroupBy: 'none',
    setProjectViewMode: vi.fn(),
    setLastProjectId: vi.fn(),
    clearLastProjectId: vi.fn(),
    setReportsPerPage: vi.fn(),
    setReportsGroupBy: vi.fn(),
    ...overrides,
  }
}

function renderSidebar(path: string, isAdmin = false, uiStateOverrides: Partial<UIState> = {}) {
  vi.mocked(useAuthStore).mockImplementation((selector: (state: AuthState) => unknown) =>
    selector({
      isAuthenticated: false,
      roles: isAdmin ? (['admin'] as Role[]) : [],
      username: null,
      expiresAt: null,
      setAuth: vi.fn(),
      clearAuth: vi.fn(),
    }),
  )
  vi.mocked(useUIStore).mockImplementation((selector: (state: UIState) => unknown) =>
    selector(makeUIState(uiStateOverrides)),
  )
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={createTestQueryClient()}>
        <TooltipProvider>
          <SidebarProvider>
            <Routes>
              <Route path="/" element={<AppSidebar />} />
              <Route path="/projects/:id/*" element={<AppSidebar />} />
              <Route path="/admin" element={<AppSidebar />} />
            </Routes>
          </SidebarProvider>
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('AppSidebar', () => {
  it('renders dashboard link with href "/"', () => {
    renderSidebar('/')
    const link = screen.getByRole('link', { name: /projects dashboard/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/')
  })

  it('does NOT render search trigger', () => {
    renderSidebar('/')
    expect(screen.queryByTestId('search-trigger')).not.toBeInTheDocument()
  })

  it('does NOT render project list items in sidebar', () => {
    renderSidebar('/')
    expect(screen.queryByText('project-alpha')).not.toBeInTheDocument()
    expect(screen.queryByText('my-project')).not.toBeInTheDocument()
  })

  it('shows "Projects" collapsible section header', () => {
    renderSidebar('/')
    expect(screen.getByText('Projects')).toBeInTheDocument()
  })

  it('shows project sub-nav when project is in URL', () => {
    renderSidebar('/projects/my-project')
    expect(screen.getByText('Overview')).toBeInTheDocument()
    expect(screen.getByText('Known Issues')).toBeInTheDocument()
    expect(screen.getByText('Timeline')).toBeInTheDocument()
    expect(screen.getByText('Analytics')).toBeInTheDocument()
  })

  it('hides project sub-nav when no project is in URL', async () => {
    vi.mocked(getProjects).mockResolvedValueOnce({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 20, total_pages: 0 },
    })
    renderSidebar('/', false, { lastProjectId: null })
    await waitFor(() => {
      expect(screen.queryByText('Overview')).not.toBeInTheDocument()
      expect(screen.queryByText('Known Issues')).not.toBeInTheDocument()
      expect(screen.queryByText('Timeline')).not.toBeInTheDocument()
      expect(screen.queryByText('Analytics')).not.toBeInTheDocument()
    })
  })

  it('nav items link to correct project paths', () => {
    renderSidebar('/projects/my-project')
    expect(screen.getByRole('link', { name: /overview/i })).toHaveAttribute(
      'href',
      '/projects/my-project',
    )
    expect(screen.getByRole('link', { name: /known issues/i })).toHaveAttribute(
      'href',
      '/projects/my-project/known-issues',
    )
    expect(screen.getByRole('link', { name: /timeline/i })).toHaveAttribute(
      'href',
      '/projects/my-project/timeline',
    )
    expect(screen.getByRole('link', { name: /analytics/i })).toHaveAttribute(
      'href',
      '/projects/my-project/analytics',
    )
  })

  it('shows "Administration" section and "System Monitor" link for admin users', () => {
    renderSidebar('/', true)
    expect(screen.getByText('Administration')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /system monitor/i })).toBeInTheDocument()
  })

  it('hides "Administration" section for non-admin users', () => {
    renderSidebar('/', false)
    expect(screen.queryByText('Administration')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /system monitor/i })).not.toBeInTheDocument()
  })

  it('displays version in footer', () => {
    renderSidebar('/')
    // In test env VITE_APP_VERSION is unset → falls back to 'dev'
    expect(screen.getByText('vdev')).toBeInTheDocument()
  })

  it('shows project sub-nav using stored lastProjectId when not on project page', () => {
    renderSidebar('/', false, { lastProjectId: 'stored-project' })
    expect(screen.getByText('Overview')).toBeInTheDocument()
    expect(screen.getByText('Known Issues')).toBeInTheDocument()
    expect(screen.getByText('Timeline')).toBeInTheDocument()
    expect(screen.getByText('Analytics')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /overview/i })).toHaveAttribute(
      'href',
      '/projects/stored-project',
    )
    expect(screen.getByRole('link', { name: /known issues/i })).toHaveAttribute(
      'href',
      '/projects/stored-project/known-issues',
    )
    expect(screen.getByRole('link', { name: /timeline/i })).toHaveAttribute(
      'href',
      '/projects/stored-project/timeline',
    )
    expect(screen.getByRole('link', { name: /analytics/i })).toHaveAttribute(
      'href',
      '/projects/stored-project/analytics',
    )
  })

  it('auto-selects first project when no stored project', async () => {
    vi.mocked(getProjects).mockResolvedValueOnce({
      data: [{ project_id: 'first-project' }],
      metadata: { message: 'ok' },
      pagination: { total: 1, page: 1, per_page: 20, total_pages: 1 },
    })
    renderSidebar('/', false, { lastProjectId: null })
    await waitFor(() => {
      expect(screen.getByText('Overview')).toBeInTheDocument()
      expect(screen.getByText('Known Issues')).toBeInTheDocument()
      expect(screen.getByText('Timeline')).toBeInTheDocument()
      expect(screen.getByText('Analytics')).toBeInTheDocument()
    })
    expect(screen.getByRole('link', { name: /overview/i })).toHaveAttribute(
      'href',
      '/projects/first-project',
    )
  })
})
