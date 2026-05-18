import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BreadcrumbBar } from '../BreadcrumbBar'
import type { ProjectEntry } from '@/types/api'

// Seed projects for tests
const parentProject: ProjectEntry = {
  project_id: 10,
  slug: 'parent-group',
  display_name: 'Parent Group',
  children: [20],
}

const childProject: ProjectEntry = {
  project_id: 20,
  slug: 'child-proj',
  display_name: 'Child Project',
  parent_id: 10,
}

const standaloneProject: ProjectEntry = {
  project_id: 30,
  slug: 'standalone',
  display_name: 'Standalone',
}

const allProjects = [parentProject, childProject, standaloneProject]

vi.mock('@/lib/resolveProject', () => ({
  useProjectFromParam: vi.fn(),
}))

vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn(),
}))

import { useProjectFromParam } from '@/lib/resolveProject'
import { fetchBranches } from '@/api/branches'
import type { Branch } from '@/types/api'

function makeBranch(name: string): Branch {
  return { id: 1, project_id: 30, name, is_default: false, created_at: '2024-01-01T00:00:00Z' }
}

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

function renderAtPath(path: string) {
  return render(
    <QueryClientProvider client={makeQueryClient()}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/" element={<BreadcrumbBar />} />
          <Route path="/projects/:id" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/analytics" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/known-issues" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/timeline" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/attachments" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/defects" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/tests" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/reports/:reportId" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/trace/:source" element={<BreadcrumbBar />} />
          <Route path="/projects/:id/compare" element={<BreadcrumbBar />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('BreadcrumbBar', () => {
  it('renders null on the dashboard route "/"', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: undefined,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    const { container } = renderAtPath('/')
    expect(container.firstChild).toBeNull()
  })

  it('renders "Projects" link pointing to "/" for a standalone project', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: standaloneProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/30')
    const projectsLink = screen.getByRole('link', { name: /projects/i })
    expect(projectsLink).toHaveAttribute('href', '/')
  })

  it('renders current project name (non-linked) for standalone project at overview', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: standaloneProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/30')
    expect(screen.getByText('Standalone')).toBeInTheDocument()
    // Should not be a link
    const allLinks = screen.getAllByRole('link')
    const projectNameLinks = allLinks.filter((l) => l.textContent?.includes('Standalone'))
    expect(projectNameLinks).toHaveLength(0)
  })

  it('renders parent segment with numeric link for child project', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: childProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/20')
    // Parent link must use numeric project_id
    const parentLink = screen.getByRole('link', { name: /parent group/i })
    expect(parentLink).toHaveAttribute('href', '/projects/10')
  })

  it('renders tab segment as non-linked text when it is the current page', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: standaloneProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/30/analytics')
    // "Analytics" should appear in the breadcrumb
    expect(screen.getByText('Analytics')).toBeInTheDocument()
    // It should NOT be a link (no href to analytics tab)
    const links = screen.getAllByRole('link')
    const analyticsLinks = links.filter((l) => l.textContent?.includes('Analytics'))
    expect(analyticsLinks).toHaveLength(0)
  })

  it('renders tab segment as a link when a deeper sub-route is open (reports)', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: standaloneProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/30/reports/42')
    // Overview tab link should point to project overview
    const overviewLink = screen.getByRole('link', { name: /overview/i })
    expect(overviewLink).toHaveAttribute('href', '/projects/30')
    // Deep segment "Report #42" should be plain text, not a link
    expect(screen.getByText(/report #42/i)).toBeInTheDocument()
    const links = screen.getAllByRole('link')
    const reportLinks = links.filter((l) => l.textContent?.match(/report #42/i))
    expect(reportLinks).toHaveLength(0)
  })

  it('renders trace source as plain text deep segment', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: standaloneProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/30/trace/my-trace.zip')
    expect(screen.getByText('my-trace.zip')).toBeInTheDocument()
  })

  it('renders compare page with "Build comparison" as plain text', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: standaloneProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/30/compare')
    expect(screen.getByText(/build comparison/i)).toBeInTheDocument()
  })

  it('shows skeleton placeholders while loading', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: undefined,
      projects: undefined,
      isLoading: true,
      error: null,
    })
    renderAtPath('/projects/30')
    // Skeleton elements rendered during loading
    expect(screen.getAllByTestId('breadcrumb-skeleton').length).toBeGreaterThan(0)
  })

  it('uses numeric project_id in all navigation links', () => {
    vi.mocked(useProjectFromParam).mockReturnValue({
      project: childProject,
      projects: allProjects,
      isLoading: false,
      error: null,
    })
    renderAtPath('/projects/20/reports/99')
    const links = screen.getAllByRole('link')
    // None of the links should contain slug strings as href
    links.forEach((link) => {
      const href = link.getAttribute('href') ?? ''
      expect(href).not.toMatch(/child-proj/)
      expect(href).not.toMatch(/parent-group/)
    })
    // The parent link uses numeric id 10
    const parentLink = screen.getByRole('link', { name: /parent group/i })
    expect(parentLink).toHaveAttribute('href', '/projects/10')
  })

  describe('BranchSelector visibility', () => {
    beforeEach(() => {
      vi.mocked(useProjectFromParam).mockReturnValue({
        project: standaloneProject,
        projects: allProjects,
        isLoading: false,
        error: null,
      })
    })

    it('shows the branch selector on the project overview route', async () => {
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30')
      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: /filter by branch/i })).toBeInTheDocument()
      })
    })

    it('shows the branch selector on the analytics route', async () => {
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30/analytics')
      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: /filter by branch/i })).toBeInTheDocument()
      })
    })

    it('shows the branch selector on the timeline route', async () => {
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30/timeline')
      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: /filter by branch/i })).toBeInTheDocument()
      })
    })

    it('shows the branch selector on the tests route', async () => {
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30/tests')
      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: /filter by branch/i })).toBeInTheDocument()
      })
    })

    it('does not show the branch selector on the defects route', async () => {
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30/defects')
      // Wait for the bar to settle, then assert the combobox is absent
      await waitFor(() => {
        expect(screen.getByText('Defects')).toBeInTheDocument()
      })
      expect(screen.queryByRole('combobox', { name: /filter by branch/i })).not.toBeInTheDocument()
    })

    it('does not show the branch selector on the attachments route', async () => {
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30/attachments')
      await waitFor(() => {
        expect(screen.getByText('Attachments')).toBeInTheDocument()
      })
      expect(screen.queryByRole('combobox', { name: /filter by branch/i })).not.toBeInTheDocument()
    })

    it('does not show the branch selector on a reports deep-link route', async () => {
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30/reports/42')
      await waitFor(() => {
        expect(screen.getByText(/report #42/i)).toBeInTheDocument()
      })
      expect(screen.queryByRole('combobox', { name: /filter by branch/i })).not.toBeInTheDocument()
    })

    it('does not show the branch selector while loading', () => {
      vi.mocked(useProjectFromParam).mockReturnValue({
        project: undefined,
        projects: undefined,
        isLoading: true,
        error: null,
      })
      vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main')])
      renderAtPath('/projects/30/analytics')
      expect(screen.queryByRole('combobox', { name: /filter by branch/i })).not.toBeInTheDocument()
    })
  })
})
