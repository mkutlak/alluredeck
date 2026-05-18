import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { SidebarProvider } from '@/components/ui/sidebar'
import { useUIStore } from '@/store/ui'
import type { UIState } from '@/store/ui'
import { ProjectSwitcher } from '../ProjectSwitcher'

// Projects fixture:
//   project_id 1 = group "Alpha Group" with children [3, 4]
//   project_id 2 = standalone leaf "my-project"
//   project_id 3 = child leaf "alpha-child-one" (parent 1)
//   project_id 4 = child leaf "alpha-child-two" (parent 1)
vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 1, slug: 'alpha-group', display_name: 'Alpha Group', parent_id: null, children: [3, 4] },
      { project_id: 2, slug: 'my-project', display_name: undefined, parent_id: null, children: [] },
      { project_id: 3, slug: 'alpha-child-one', display_name: undefined, parent_id: 1, children: [] },
      { project_id: 4, slug: 'alpha-child-two', display_name: undefined, parent_id: 1, children: [] },
    ],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn().mockResolvedValue({
    data: [
      { project_id: 1, slug: 'alpha-group', display_name: 'Alpha Group', parent_id: null, children: [3, 4] },
      { project_id: 2, slug: 'my-project', display_name: undefined, parent_id: null, children: [] },
      { project_id: 3, slug: 'alpha-child-one', display_name: undefined, parent_id: 1, children: [] },
      { project_id: 4, slug: 'alpha-child-two', display_name: undefined, parent_id: 1, children: [] },
    ],
    metadata: { message: 'ok' },
    pagination: { total: 4, page: 1, per_page: 20, total_pages: 1 },
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
const mockPinProject = vi.fn()
const mockUnpinProject = vi.fn()

beforeEach(() => {
  mockSetLastProjectId.mockClear()
  mockNavigate.mockClear()
  mockPinProject.mockClear()
  mockUnpinProject.mockClear()
})

interface RenderOptions {
  lastProjectId?: string | null
  pinnedProjectIds?: number[]
  recentProjectIds?: number[]
  lastTabPerProject?: Record<string, string>
}

function buildUIState(opts: RenderOptions = {}): UIState {
  return {
    lastProjectId: opts.lastProjectId ?? null,
    setLastProjectId: mockSetLastProjectId,
    clearLastProjectId: vi.fn(),
    projectViewMode: 'grid',
    setProjectViewMode: vi.fn(),
    reportsPerPage: 20,
    reportsGroupBy: 'none' as const,
    selectedBranch: undefined,
    _syncedAt: null,
    timezone: null,
    timeFormat: null,
    setReportsPerPage: vi.fn(),
    setReportsGroupBy: vi.fn(),
    setSelectedBranch: vi.fn(),
    setSyncedAt: vi.fn(),
    setTimezone: vi.fn(),
    setTimeFormat: vi.fn(),
    pinnedProjectIds: opts.pinnedProjectIds ?? [],
    recentProjectIds: opts.recentProjectIds ?? [],
    lastTabPerProject: opts.lastTabPerProject ?? {},
    pinProject: mockPinProject,
    unpinProject: mockUnpinProject,
    recordProjectVisit: vi.fn(),
    setLastTabForProject: vi.fn(),
  }
}

function renderSwitcher(path: string, opts: RenderOptions = {}) {
  const state = buildUIState(opts)

  // Also expose state on the mock's getState so the component can call
  // useUIStore.getState().lastTabPerProject
  const storeMock = vi.mocked(useUIStore)
  storeMock.mockImplementation((selector: (s: UIState) => unknown) => selector(state))
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ;(storeMock as any).getState = () => state

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
  // ── Trigger label ──────────────────────────────────────────────────────────

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

  it('shows stored project name when not on project page', async () => {
    renderSwitcher('/', { lastProjectId: 'my-project' })
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /my-project/i })).toBeInTheDocument()
    })
  })

  // ── Dropdown opens ─────────────────────────────────────────────────────────

  it('opens a dropdown when clicked', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search project/i)).toBeInTheDocument()
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

  // ── Recents section ────────────────────────────────────────────────────────

  it('shows Recents section when recentProjectIds is non-empty', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { recentProjectIds: [2] })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('Recents')).toBeInTheDocument()
      // my-project (id=2) should appear in recents
      expect(screen.getAllByText('my-project').length).toBeGreaterThan(0)
    })
  })

  it('does not show Recents section when recentProjectIds is empty', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { recentProjectIds: [] })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.queryByText('Recents')).not.toBeInTheDocument()
    })
  })

  it('skips recent ids no longer in the project list', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { recentProjectIds: [999] })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      // 999 does not exist — Recents section should not appear
      expect(screen.queryByText('Recents')).not.toBeInTheDocument()
    })
  })

  // ── Pinned section ─────────────────────────────────────────────────────────

  it('shows Pinned section when pinnedProjectIds is non-empty', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { pinnedProjectIds: [2] })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('Pinned')).toBeInTheDocument()
    })
  })

  it('does not show Pinned section when pinnedProjectIds is empty', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { pinnedProjectIds: [] })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.queryByText('Pinned')).not.toBeInTheDocument()
    })
  })

  // ── All Projects: hierarchy ────────────────────────────────────────────────

  it('shows All Projects heading', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('All Projects')).toBeInTheDocument()
    })
  })

  it('shows group header (non-selectable) and indented children in All Projects', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      const listbox = screen.getByRole('listbox')
      expect(within(listbox).getAllByText('Alpha Group').length).toBeGreaterThan(0)
      expect(within(listbox).getByText('Alpha Group/alpha-child-one')).toBeInTheDocument()
      expect(within(listbox).getByText('Alpha Group/alpha-child-two')).toBeInTheDocument()
    })
  })

  it('group headers are disabled (not keyboard-selectable)', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      const listbox = screen.getByRole('listbox')
      // The group header's CommandItem text is in a span with class "flex-1 truncate font-medium"
      const groupHeaderSpan = within(listbox).getByText('Alpha Group', {
        selector: '.flex-1.truncate.font-medium',
      })
      const groupItem = groupHeaderSpan.closest('[role="option"]')
      expect(groupItem).toHaveAttribute('aria-disabled', 'true')
    })
  })

  it('clicking a group header does not navigate', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByRole('listbox')).toBeInTheDocument()
    })
    const listbox = screen.getByRole('listbox')
    const groupHeaderSpan = within(listbox).getByText('Alpha Group', {
      selector: '.flex-1.truncate.font-medium',
    })
    // Click the disabled group item — navigate should NOT be called
    const groupEl = groupHeaderSpan.closest('[role="option"]')
    if (groupEl) await user.click(groupEl)
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('shows standalone leaf projects flat (no parent group)', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      // my-project is standalone
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
  })

  // ── Navigation ─────────────────────────────────────────────────────────────

  it('navigates to /projects/<id> when no lastTab is stored', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { lastTabPerProject: {} })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
    await user.click(screen.getByText('my-project'))
    expect(mockNavigate).toHaveBeenCalledWith('/projects/2')
  })

  it('navigates to /projects/<id>/<tab> when lastTabPerProject has an entry', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { lastTabPerProject: { '2': 'analytics' } })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
    await user.click(screen.getByText('my-project'))
    expect(mockNavigate).toHaveBeenCalledWith('/projects/2/analytics')
  })

  it('navigates to child project using numeric project_id', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('Alpha Group/alpha-child-one')).toBeInTheDocument()
    })
    await user.click(screen.getByText('Alpha Group/alpha-child-one'))
    expect(mockNavigate).toHaveBeenCalledWith('/projects/3')
  })

  it('closes the dropdown after selecting a project', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
    await user.click(screen.getByText('my-project'))
    await waitFor(() => {
      expect(screen.queryByPlaceholderText(/search project/i)).not.toBeInTheDocument()
    })
  })

  it('updates lastProjectId in store when selecting a project', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
    await user.click(screen.getByText('my-project'))
    expect(mockSetLastProjectId).toHaveBeenCalledWith('2')
  })

  // ── Pin toggle ─────────────────────────────────────────────────────────────

  it('clicking the star pin button calls pinProject and does not navigate', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { pinnedProjectIds: [] })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
    const pinBtn = screen.getByRole('button', { name: /pin my-project/i })
    await user.click(pinBtn)
    expect(mockPinProject).toHaveBeenCalledWith(2)
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('clicking the star on a pinned project calls unpinProject', async () => {
    const user = userEvent.setup()
    renderSwitcher('/', { pinnedProjectIds: [2] })
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      // my-project appears in Pinned section and All Projects
      expect(screen.getByText('Pinned')).toBeInTheDocument()
    })
    const unpinBtns = screen.getAllByRole('button', { name: /unpin my-project/i })
    await user.click(unpinBtns[0])
    expect(mockUnpinProject).toHaveBeenCalledWith(2)
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  // ── Search mode ────────────────────────────────────────────────────────────

  it('shows flat leaf-only list when search input is non-empty', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search project/i)).toBeInTheDocument()
    })
    await user.type(screen.getByPlaceholderText(/search project/i), 'alpha')
    await waitFor(() => {
      // Group headings should disappear
      expect(screen.queryByText('All Projects')).not.toBeInTheDocument()
      expect(screen.queryByText('Recents')).not.toBeInTheDocument()
    })
  })

  it('does not include group headers in search results', async () => {
    const user = userEvent.setup()
    renderSwitcher('/')
    await user.click(screen.getByRole('button', { name: /select a project/i }))
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search project/i)).toBeInTheDocument()
    })
    await user.type(screen.getByPlaceholderText(/search project/i), 'alpha')
    // Wait for filter to apply, then verify group header text not in an option role
    await waitFor(() => {
      const options = screen.queryAllByRole('option', { name: /Alpha Group$/ })
      // The standalone "Alpha Group" option (group header) should not be selectable
      const selectable = options.filter((o) => o.getAttribute('aria-disabled') !== 'true')
      expect(selectable).toHaveLength(0)
    })
  })
})
