import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { BreadcrumbBar } from '../BreadcrumbBar'
import type { ProjectEntry } from '@/types/api'

vi.mock('@/lib/resolveProject', () => ({
  useProjectFromParam: vi.fn(),
}))

import { useProjectFromParam } from '@/lib/resolveProject'

const parent: ProjectEntry = { project_id: 10, slug: 'parent-group', display_name: 'Parent Group' }
const child: ProjectEntry = {
  project_id: 20,
  slug: 'child-proj',
  display_name: 'Child Project',
  parent_id: 10,
}

function renderAt(path: string, resolved: Partial<ReturnType<typeof useProjectFromParam>>) {
  vi.mocked(useProjectFromParam).mockReturnValue({
    project: undefined,
    projects: [parent, child],
    isLoading: false,
    error: null,
    ...resolved,
  })
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/projects/:id/reports/:reportId" element={<BreadcrumbBar />} />
        <Route path="/projects/:id" element={<BreadcrumbBar />} />
      </Routes>
    </MemoryRouter>,
  )
}

// Crumb derivation is covered by breadcrumbs.test.ts; this pins the wiring.
describe('BreadcrumbBar', () => {
  it('links every crumb but the current page, from the route and resolved project', () => {
    renderAt('/projects/child-proj/reports/99', { project: child })

    const nav = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(
      within(nav)
        .getAllByRole('link')
        .map((a) => [a.textContent, a.getAttribute('href')]),
    ).toEqual([
      ['Projects', '/'],
      ['Parent Group', '/projects/10'],
      ['Overview', '/projects/20'],
    ])
    expect(nav).toHaveTextContent(/Child Project.*Report #99$/)
  })

  it('shows skeleton placeholders while loading', () => {
    renderAt('/projects/30', { isLoading: true })
    expect(screen.getAllByTestId('breadcrumb-skeleton').length).toBeGreaterThan(0)
  })
})
