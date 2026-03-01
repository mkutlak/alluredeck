import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SidebarProvider } from '@/components/ui/sidebar'
import * as projectsApi from '@/api/projects'
import { ProjectSwitcher } from '../ProjectSwitcher'

vi.mock('@/api/projects')
vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
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

function renderSwitcher(path = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={qc}>
        <SidebarProvider>
          <Routes>
            <Route path="/" element={<ProjectSwitcher />} />
            <Route path="/projects/:id/*" element={<ProjectSwitcher />} />
          </Routes>
        </SidebarProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

const mockProjectsResponse = {
  data: [
    { project_id: 'proj-a', created_at: '2026-01-01T00:00:00Z' },
    { project_id: 'proj-b', created_at: '2026-01-02T00:00:00Z' },
  ],
  metadata: { message: 'ok' },
  pagination: { page: 1, per_page: 20, total: 2, total_pages: 1 },
}

describe('ProjectSwitcher', () => {
  it('shows "Select project…" when no project is active', () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue(mockProjectsResponse)
    renderSwitcher('/')
    expect(screen.getByText('Select project…')).toBeInTheDocument()
  })

  it('shows the active project name when a project is selected', () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue(mockProjectsResponse)
    renderSwitcher('/projects/proj-a')
    expect(screen.getByText('proj-a')).toBeInTheDocument()
  })

  it('opens the command popover on click and lists projects', async () => {
    const user = userEvent.setup()
    vi.mocked(projectsApi.getProjects).mockResolvedValue(mockProjectsResponse)
    renderSwitcher('/')

    await user.click(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search projects/i)).toBeInTheDocument()
      expect(screen.getByText('proj-a')).toBeInTheDocument()
      expect(screen.getByText('proj-b')).toBeInTheDocument()
    })
  })

  it('shows "All projects" option in the list', async () => {
    const user = userEvent.setup()
    vi.mocked(projectsApi.getProjects).mockResolvedValue(mockProjectsResponse)
    renderSwitcher('/')

    await user.click(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('All projects')).toBeInTheDocument()
    })
  })
})
