import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ProjectLayout } from '../ProjectLayout'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 1, slug: 'parent-project', display_name: 'Parent Project', children: [2] },
      {
        project_id: 2,
        slug: 'child-project',
        display_name: 'Child Project',
        parent_id: 1,
        children: [],
      },
      { project_id: 3, slug: 'solo-project', display_name: 'Solo Project', children: [] },
    ],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn().mockResolvedValue({
    data: [
      { project_id: 1, slug: 'parent-project', display_name: 'Parent Project', children: [2] },
      {
        project_id: 2,
        slug: 'child-project',
        display_name: 'Child Project',
        parent_id: 1,
        children: [],
      },
      { project_id: 3, slug: 'solo-project', display_name: 'Solo Project', children: [] },
    ],
    metadata: { message: 'ok' },
    pagination: { total: 3, page: 1, per_page: 20, total_pages: 1 },
  }),
}))

import { mockApiClient } from '@/test/mocks/api-client'
mockApiClient()

vi.mock('@/store/auth', () => ({
  useAuthStore: () => false,
  selectIsAdmin: () => false,
  selectIsEditor: () => false,
}))

function renderLayout(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={createTestQueryClient()}>
        <TooltipProvider>
          <Routes>
            <Route path="/projects/:id" element={<ProjectLayout />}>
              <Route index element={<div data-testid="outlet-content">Outlet content</div>} />
              <Route path="analytics" element={<div>Analytics content</div>} />
            </Route>
          </Routes>
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('ProjectLayout', () => {
  it('renders a loading skeleton while the project resolves', () => {
    renderLayout('/projects/3')
    expect(screen.getAllByTestId('project-layout-skeleton').length).toBeGreaterThan(0)
  })

  it('renders the mono project title once resolved', async () => {
    renderLayout('/projects/3')
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Solo Project' })).toBeInTheDocument()
    })
  })

  it('renders "Part of: <parent>" subtitle for a child project, linking by numeric id', async () => {
    renderLayout('/projects/2')
    await waitFor(() => {
      expect(screen.getByText(/Part of:/)).toBeInTheDocument()
    })
    const link = screen.getByRole('link', { name: 'parent-project' })
    expect(link).toHaveAttribute('href', '/projects/1')
  })

  it('does not render a "Part of" subtitle for a project without a parent', async () => {
    renderLayout('/projects/3')
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Solo Project' })).toBeInTheDocument()
    })
    expect(screen.queryByText(/Part of:/)).not.toBeInTheDocument()
  })

  it('renders TabsNav with project section links', async () => {
    renderLayout('/projects/3')
    await waitFor(() => {
      expect(screen.getByRole('navigation', { name: 'Project sections' })).toBeInTheDocument()
    })
    expect(screen.getByTestId('sidebar-nav-overview')).toBeInTheDocument()
    expect(screen.getByTestId('sidebar-nav-analytics')).toBeInTheDocument()
  })

  it('renders "Pipeline Runs" tab only for a parent project', async () => {
    renderLayout('/projects/1')
    await waitFor(() => {
      expect(screen.getByRole('navigation', { name: 'Project sections' })).toBeInTheDocument()
    })
    expect(screen.getByText('Pipeline Runs')).toBeInTheDocument()
    expect(screen.queryByTestId('sidebar-nav-analytics')).not.toBeInTheDocument()
  })

  it('renders the routed Outlet content', async () => {
    renderLayout('/projects/3')
    await waitFor(() => {
      expect(screen.getByTestId('outlet-content')).toBeInTheDocument()
    })
  })
})
