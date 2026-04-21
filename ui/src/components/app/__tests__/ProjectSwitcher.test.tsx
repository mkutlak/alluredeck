import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { SidebarProvider } from '@/components/ui/sidebar'
import { useUIStore } from '@/store/ui'
import type { UIState } from '@/store/ui'
import { ProjectSwitcher } from '../ProjectSwitcher'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 1, slug: 'project-alpha' }, { project_id: 2, slug: 'my-project' }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn().mockResolvedValue({
    data: [{ project_id: 1, slug: 'project-alpha' }, { project_id: 2, slug: 'my-project' }],
    metadata: { message: 'ok' },
    pagination: { total: 2, page: 1, per_page: 20, total_pages: 1 },
  }),
}))

import { mockApiClient } from '@/test/mocks/api-client'
mockApiClient()

vi.mock('@/store/ui', () => ({
  useUIStore: vi.fn(),
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

const mockNavigate = vi.fn()
vi.mock('react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router')>()
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

const mockSetLastProjectId = vi.fn()

beforeEach(() => {
  mockSetLastProjectId.mockClear()
})

function renderSwitcher(path: string, lastProjectId: string | null = null) {
  vi.mocked(useUIStore).mockImplementation((selector: (s: UIState) => unknown) =>
    selector({
      lastProjectId,
      setLastProjectId: mockSetLastProjectId,
      clearLastProjectId: vi.fn(),
      projectViewMode: 'grid',
      setProjectViewMode: vi.fn(),
      reportsPerPage: 20,
      reportsGroupBy: 'none' as const,
      selectedBranch: undefined,
      _syncedAt: null,
      setReportsPerPage: vi.fn(),
      setReportsGroupBy: vi.fn(),
      setSelectedBranch: vi.fn(),
      setSyncedAt: vi.fn(),
    }),
  )
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={createTestQueryClient()}>
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

describe('ProjectSwitcher', () => {
  it('renders "Select a project..." when no project is selected', () => {
    renderSwitcher('/')
    expect(screen.getByRole('button', { name: /select a project/i })).toBeInTheDocument()
  })

  it('renders the current project name when on a project page', async () => {
    renderSwitcher('/projects/my-project')
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /my-project/i })).toBeInTheDocument()
    })
  })

  it('opens a dropdown when clicked', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    const trigger = screen.getByRole('button', { name: /select a project/i })
    await user.click(trigger)
    // Popover content should be visible
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search project/i)).toBeInTheDocument()
    })
  })

  it('lists all projects in the dropdown', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'project-alpha' })).toBeInTheDocument()
      expect(screen.getByRole('option', { name: 'my-project' })).toBeInTheDocument()
    })
  })

  it('shows a search input in the dropdown', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search project/i)).toBeInTheDocument()
    })
  })

  it('navigates to a project and closes the dropdown when a project item is clicked', async () => {
    const user = userEvent.setup()
    mockNavigate.mockClear()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'project-alpha' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('option', { name: 'project-alpha' }))
    expect(mockNavigate).toHaveBeenCalledWith('/projects/1')
    // Popover should close — search input no longer visible
    await waitFor(() => {
      expect(screen.queryByPlaceholderText(/search project/i)).not.toBeInTheDocument()
    })
  })

  it('shows stored project name when not on project page', async () => {
    renderSwitcher('/', 'my-project')
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /my-project/i })).toBeInTheDocument()
    })
  })

  it('selecting a project updates lastProjectId in store', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'project-alpha' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('option', { name: 'project-alpha' }))
    expect(mockSetLastProjectId).toHaveBeenCalledWith('1')
  })
})
