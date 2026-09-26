import { describe, it, expect, vi } from 'vitest'
import { screen, within } from '@testing-library/react'
import { Route, Routes } from 'react-router'
import { renderWithProviders } from '@/test/render'
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
}))

function renderLayout(path: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/projects/:id" element={<ProjectLayout />}>
        <Route index element={<div data-testid="outlet-content">Outlet content</div>} />
      </Route>
    </Routes>,
    { route: path },
  )
}

describe('ProjectLayout', () => {
  it('shows a skeleton until the project resolves, then its title, tabs and outlet', async () => {
    renderLayout('/projects/3')
    expect(screen.getAllByTestId('project-layout-skeleton').length).toBeGreaterThan(0)

    expect(await screen.findByRole('heading', { name: 'Solo Project' })).toBeInTheDocument()
    expect(screen.queryByText(/Part of:/)).not.toBeInTheDocument()
    const tabs = screen.getByRole('navigation', { name: 'Project sections' })
    expect(within(tabs).getByRole('link', { name: 'Analytics' })).toHaveAttribute(
      'href',
      '/projects/3/analytics',
    )
    expect(screen.getByTestId('outlet-content')).toBeInTheDocument()
  })

  it('renders "Part of: <parent>" subtitle for a child project, linking by numeric id', async () => {
    renderLayout('/projects/2')
    expect(await screen.findByText(/Part of:/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'parent-project' })).toHaveAttribute(
      'href',
      '/projects/1',
    )
  })
})
