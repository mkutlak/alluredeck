import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { ProjectsPage } from './ProjectsPage'
import * as projectsApi from '@/api/projects'
import { useAuthStore } from '@/store/auth'

vi.mock('@/api/projects')
vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function renderPage(isAdminUser = true) {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: isAdminUser ? ['admin'] : ['viewer'],
    username: isAdminUser ? 'admin' : 'viewer',
    expiresAt: Date.now() + 3_600_000,
  })

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ProjectsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ProjectsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeletons initially', () => {
    vi.mocked(projectsApi.getProjects).mockReturnValue(new Promise(() => {}))
    renderPage()
    // Loading skeletons rendered — no heading yet for project count
    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('renders project cards when loaded', async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: {
        'my-project': { project_id: 'my-project' },
        'other-project': { project_id: 'other-project' },
      },
      meta_data: { message: 'ok' },
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
      data: {},
      meta_data: { message: 'ok' },
    })

    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/no projects yet/i)).toBeInTheDocument()
    })
  })

  it('shows "New project" button for admin', async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: {},
      meta_data: { message: 'ok' },
    })

    renderPage(true)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /new project/i })).toBeInTheDocument()
    })
  })

  it('hides "New project" button for viewer', async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: {},
      meta_data: { message: 'ok' },
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

  it('opens create dialog when "New project" is clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(projectsApi.getProjects).mockResolvedValue({
      data: {},
      meta_data: { message: 'ok' },
    })

    renderPage(true)
    await waitFor(() => screen.getByRole('button', { name: /new project/i }))
    await user.click(screen.getByRole('button', { name: /new project/i }))

    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })
  })
})
