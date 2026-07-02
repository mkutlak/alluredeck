import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { SidebarProvider } from '@/components/ui/sidebar'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useAuthStore } from '@/store/auth'
import type { AuthState, Role } from '@/store/auth'
import { AppSidebar } from '../AppSidebar'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 'project-alpha' }, { project_id: 'my-project' }],
    metadata: { message: 'ok' },
  }),
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
  selectIsEditor: (s: { roles?: string[] }) => {
    const r = s.roles ?? []
    return r.includes('admin') || r.includes('editor')
  },
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

function renderSidebar(path: string, roles: Role[] = []) {
  vi.mocked(useAuthStore).mockImplementation((selector: (state: AuthState) => unknown) =>
    selector({
      isAuthenticated: false,
      roles,
      username: null,
      expiresAt: null,
      provider: null,
      setAuth: vi.fn(),
      clearAuth: vi.fn(),
    }),
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
    const link = screen.getByRole('link', { name: /projects/i })
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

  it('shows "Projects" nav link in sidebar', () => {
    renderSidebar('/')
    expect(screen.getByText('Projects')).toBeInTheDocument()
  })

  it('does NOT render a project sub-nav section, even when on a project page', () => {
    renderSidebar('/projects/my-project')
    expect(screen.queryByText('Overview')).not.toBeInTheDocument()
    expect(screen.queryByText('Known Issues')).not.toBeInTheDocument()
    expect(screen.queryByText('Timeline')).not.toBeInTheDocument()
    expect(screen.queryByText('Analytics')).not.toBeInTheDocument()
    expect(screen.queryByTestId('sidebar-nav-overview')).not.toBeInTheDocument()
  })

  it('shows "Administration" section and "System Monitor" link for admin users', () => {
    renderSidebar('/', ['admin'])
    expect(screen.getByText('Administration')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /system monitor/i })).toBeInTheDocument()
  })

  it('hides "System Monitor" link for non-admin users', () => {
    renderSidebar('/')
    expect(screen.getByText('Administration')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /system monitor/i })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /api keys/i })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /webhooks/i })).not.toBeInTheDocument()
  })

  it('shows "Webhooks" link for editor users', () => {
    renderSidebar('/', ['editor'])
    expect(screen.getByRole('link', { name: /webhooks/i })).toBeInTheDocument()
  })

  it('displays version in footer', () => {
    renderSidebar('/')
    // In test env VITE_APP_VERSION is unset → falls back to 'dev'
    expect(screen.getByText('vdev')).toBeInTheDocument()
  })
})
