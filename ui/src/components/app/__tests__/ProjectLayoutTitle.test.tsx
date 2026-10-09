import { screen, waitFor } from '@testing-library/react'
import { Route, Routes } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { ProjectLayout } from '../ProjectLayout'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 1, slug: 'parent-project', display_name: 'Parent Project', children: [3] },
      { project_id: 3, slug: 'solo-project', display_name: 'Solo Project', children: [] },
      { project_id: 4, slug: 'my project', display_name: 'Spaced', children: [] },
      { project_id: 5, slug: 'café', display_name: 'Cafe', children: [] },
    ],
    metadata: { message: 'ok' },
  }),
}))

beforeEach(() => {
  document.title = 'AllureDeck'
})

function renderLayoutAt(path: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/projects/:id" element={<ProjectLayout />}>
        <Route index element={null} />
        <Route path="analytics" element={null} />
        <Route path="reports/:reportId" element={null} />
      </Route>
    </Routes>,
    { route: path },
  )
}

describe('ProjectLayout document title', () => {
  // Tabs link by numeric id; a typed URL may use the slug, percent-encoded in the path.
  // Routes match regardless of case and trailing slashes, so the title does too.
  it.each([
    { path: '/projects/3/analytics', title: 'Analytics · Solo Project — AllureDeck' },
    { path: '/projects/solo-project', title: 'Overview · Solo Project — AllureDeck' },
    { path: '/projects/1', title: 'Pipeline Runs · Parent Project — AllureDeck' },
    { path: '/projects/my%20project/analytics', title: 'Analytics · Spaced — AllureDeck' },
    { path: '/projects/caf%C3%A9/analytics', title: 'Analytics · Cafe — AllureDeck' },
    { path: '/projects/3/Analytics', title: 'Analytics · Solo Project — AllureDeck' },
    { path: '/projects/3/analytics/', title: 'Analytics · Solo Project — AllureDeck' },
    { path: '/projects/3/analytics//', title: 'Analytics · Solo Project — AllureDeck' },
  ])('names the tab "$title" at $path', async ({ path, title }) => {
    const { unmount } = renderLayoutAt(path)
    await screen.findByRole('heading', { level: 1 })
    await waitFor(() => expect(document.title).toBe(title))
    unmount()
    expect(document.title).toBe('AllureDeck')
  })

  // Not a tab: the layout leaves the title to the page.
  it('leaves the title alone on a page below a tab', async () => {
    renderLayoutAt('/projects/3/reports/5')
    await screen.findByRole('heading', { level: 1 })
    expect(document.title).toBe('AllureDeck')
  })
})
