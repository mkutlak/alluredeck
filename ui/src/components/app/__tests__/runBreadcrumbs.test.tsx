import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { BreadcrumbBar } from '../BreadcrumbBar'
import { buildBreadcrumbs, LOADING_CRUMB } from '../breadcrumbs'
import type { ProjectEntry } from '@/types/api'

vi.mock('@/lib/resolveProject', () => ({
  useProjectFromParam: vi.fn(),
}))

import { useProjectFromParam } from '@/lib/resolveProject'

const group: ProjectEntry = { project_id: 10, slug: 'acme', display_name: 'Acme Payments' }
const projects = [group]

const trail = (runKey: string, project: ProjectEntry | undefined, isLoading = false) =>
  buildBreadcrumbs(
    `/runs/10/${encodeURIComponent(runKey)}`,
    { groupId: '10', runKey },
    { project, projects, isLoading },
  )?.map((c) => (c.href ? `${c.label} -> ${c.href}` : c.label)) ?? null

describe('run page breadcrumbs', () => {
  it.each([
    { name: 'a pipeline ID as is', key: '1234567890', crumb: '1234567890' },
    // The key is the commit SHA when there is no pipeline ID; the page shows it short.
    { name: 'a commit SHA short', key: 'a1b2c3d4e5f60718293a', crumb: 'a1b2c3d' },
  ])('reads Runs / group / run, naming $name', ({ key, crumb }) => {
    expect(trail(key, group)).toEqual(['Runs -> /', 'Acme Payments -> /projects/10', crumb])
  })

  it('falls back to the group id from the URL when the group does not resolve', () => {
    expect(trail('48213', undefined)).toEqual(['Runs -> /', '10 -> /projects/10', '48213'])
  })

  it('returns the loading sentinel while the group resolves', () => {
    expect(trail('48213', undefined, true)).toEqual([LOADING_CRUMB])
  })

  it('resolves the group from the route and links Runs and the group', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: group,
      projects,
      isLoading: false,
      error: null,
    })
    render(
      <MemoryRouter initialEntries={['/runs/10/48213']}>
        <Routes>
          <Route path="/runs/:groupId/:runKey" element={<BreadcrumbBar />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(useProjectFromParam).toHaveBeenCalledWith('10')
    const nav = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(
      within(nav)
        .getAllByRole('link')
        .map((a) => [a.textContent, a.getAttribute('href')]),
    ).toEqual([
      ['Runs', '/'],
      ['Acme Payments', '/projects/10'],
    ])
    expect(nav).toHaveTextContent(/48213$/)
  })
})
