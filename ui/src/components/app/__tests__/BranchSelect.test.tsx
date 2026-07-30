import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { useUIStore } from '@/store/ui'
import type { UIState } from '@/store/ui'
import * as branchesApi from '@/api/branches'
import type { Branch } from '@/types/api'
import { BranchSelect } from '../BranchSelect'

vi.mock('@/api/branches')

vi.mock('@/store/ui', () => ({
  useUIStore: vi.fn(),
}))

function makeBranch(overrides: Partial<Branch> = {}): Branch {
  return {
    id: 1,
    project_id: 1,
    name: 'main',
    is_default: false,
    created_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

const mockSetSelectedBranch = vi.fn()

function makeUIState(overrides: Partial<UIState> = {}): UIState {
  return {
    projectViewMode: 'grid',
    lastProjectId: null,
    reportsPerPage: 20,
    reportsGroupBy: 'none',
    selectedBranch: undefined,
    _syncedAt: null,
    timezone: null,
    timeFormat: null,
    setProjectViewMode: vi.fn(),
    setLastProjectId: vi.fn(),
    clearLastProjectId: vi.fn(),
    setReportsPerPage: vi.fn(),
    setReportsGroupBy: vi.fn(),
    setSelectedBranch: mockSetSelectedBranch,
    setSyncedAt: vi.fn(),
    setTimezone: vi.fn(),
    setTimeFormat: vi.fn(),
    pinnedProjectIds: [],
    recentProjectIds: [],
    lastTabPerProject: {},
    runsFeedGroupIds: [],
    runsFailureGrouping: 'suite',
    pinProject: vi.fn(),
    unpinProject: vi.fn(),
    recordProjectVisit: vi.fn(),
    setLastTabForProject: vi.fn(),
    setRunsFeedGroupIds: vi.fn(),
    setRunsFailureGrouping: vi.fn(),
    ...overrides,
  }
}

function renderBranchSelect(path: string, uiStateOverrides: Partial<UIState> = {}) {
  vi.mocked(useUIStore).mockImplementation((selector: (s: UIState) => unknown) =>
    selector(makeUIState(uiStateOverrides)),
  )
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={createTestQueryClient()}>
        <Routes>
          <Route path="/projects/:id" element={<BranchSelect />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('BranchSelect', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('reads selectedBranch from the UI store and shows it', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ name: 'main' }),
      makeBranch({ id: 2, name: 'dev' }),
    ])
    renderBranchSelect('/projects/1', { selectedBranch: 'dev' })
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveTextContent('dev')
    })
  })

  it('calls setSelectedBranch from the UI store on selection', async () => {
    const user = userEvent.setup()
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ name: 'main' }),
      makeBranch({ id: 2, name: 'dev' }),
    ])
    renderBranchSelect('/projects/1')
    await waitFor(() => screen.getByRole('combobox'))
    await user.click(screen.getByRole('combobox'))
    const devOption = await screen.findByRole('option', { name: 'dev' })
    await user.click(devOption)
    expect(mockSetSelectedBranch).toHaveBeenCalledWith('dev')
  })

  it('renders nothing when the project has no branches', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([])
    const { container } = renderBranchSelect('/projects/1')
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })
})
