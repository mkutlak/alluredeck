import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import { ProjectsPage } from './ProjectsPage'
import * as projectsApi from '@/api/projects'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/projects')
mockApiClient()

function renderPage(isAdminUser = true) {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: isAdminUser ? ['admin'] : ['viewer'],
    username: isAdminUser ? 'admin' : 'viewer',
    expiresAt: Date.now() + 3_600_000,
  })

  return renderWithProviders(<ProjectsPage />)
}

describe('ProjectsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useUIStore.setState({ projectViewMode: 'grid' })
  })

  it('shows loading skeletons initially', () => {
    vi.mocked(projectsApi.getProjects).mockReturnValue(new Promise(() => {}))
    renderPage()
    // Loading skeletons rendered — no heading yet for project count
    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('renders project cards when loaded', async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: [{ project_id: 1, slug: 'my-project' }, { project_id: 2, slug: 'other-project' }],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 20, total: 2, total_pages: 1 },
    })

    renderPage()
    await waitFor(() => {
      // ProjectCard renders the project ID in both CardTitle and Badge — use getAllByText
      expect(screen.getAllByText('my-project').length).toBeGreaterThan(0)
      expect(screen.getAllByText('other-project').length).toBeGreaterThan(0)
    })
  })

  it('shows empty state when no projects', async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
    })

    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/no projects yet/i)).toBeInTheDocument()
    })
  })

  it('shows "New project" button for admin', async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
    })

    renderPage(true)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /new project/i })).toBeInTheDocument()
    })
  })

  it('hides "New project" button for viewer', async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
    })

    renderPage(false)
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /new project/i })).not.toBeInTheDocument()
    })
  })

  it('shows error state on API failure', async () => {
    vi.mocked(projectsApi.getProjects).mockRejectedValue(new Error('Network Error'))

    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    })
  })

  it('shows View reports Link with correct href in table view', async () => {
    useUIStore.setState({ projectViewMode: 'table' })
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: [{ project_id: 1, slug: 'my-project' }],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
    })
    renderPage()
    await waitFor(() => {
      const link = screen.getByRole('link', { name: /view reports/i })
      expect(link).toHaveAttribute('href', '/projects/1')
    })
  })

  it('opens create dialog when "New project" is clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 20, total: 0, total_pages: 0 },
    })

    renderPage(true)
    await waitFor(() => screen.getByRole('button', { name: /new project/i }))
    await user.click(screen.getByRole('button', { name: /new project/i }))

    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })
  })
})
