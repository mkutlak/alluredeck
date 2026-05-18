import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import * as dashboardApi from '@/api/dashboard'

vi.mock('recharts', () => ({
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  LineChart: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Line: () => null,
}))

vi.mock('@/api/dashboard')
import { mockApiClient } from '@/test/mocks/api-client'
mockApiClient()
vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
}))
vi.mock('@/features/projects/CreateProjectDialog', () => ({
  CreateProjectDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="create-dialog" /> : null,
}))

// Import AFTER mocks
import { DashboardPage } from '../DashboardPage'
import { useAuthStore } from '@/store/auth'
import type { DashboardData } from '@/types/api'
import type { AuthState } from '@/store/auth'

type AuthSelector = (s: Partial<AuthState>) => unknown

const mockData: DashboardData = {
  projects: [
    {
      project_id: 1,
      slug: 'proj-alpha',
      created_at: '2025-01-01T00:00:00Z',
      latest_build: null,
      sparkline: [],
    },
    {
      project_id: 3,
      slug: 'group-one',
      created_at: '2025-01-03T00:00:00Z',
      latest_build: null,
      sparkline: [],
      is_group: true,
      aggregate: { passed: 72, failed: 8, broken: 2, skipped: 0, total: 82, pass_rate: 87.8 },
      children: [
        {
          project_id: 4,
          slug: 'child-a',
          created_at: '2025-01-03T00:00:00Z',
          latest_build: null,
          sparkline: [],
        },
        {
          project_id: 5,
          slug: 'child-b',
          created_at: '2025-01-03T00:00:00Z',
          latest_build: null,
          sparkline: [],
        },
      ],
    },
  ],
  summary: { total_projects: 3, healthy: 0, degraded: 0, failing: 0 },
}

function renderPage(route = '/') {
  return renderWithProviders(<DashboardPage />, { route })
}

describe('Dashboard tree view — grouped mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: [] }),
    )
  })

  it('group row renders a chevron button', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('group-one')).toBeInTheDocument()
    })
    // The chevron should be a button with an accessible label
    expect(screen.getByRole('button', { name: /expand group-one/i })).toBeInTheDocument()
  })

  it('child rows are hidden before expanding', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('group-one')).toBeInTheDocument()
    })
    expect(screen.queryByText('child-a')).not.toBeInTheDocument()
    expect(screen.queryByText('child-b')).not.toBeInTheDocument()
  })

  it('clicking the chevron reveals indented child rows', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('group-one')).toBeInTheDocument()
    })

    const chevron = screen.getByRole('button', { name: /expand group-one/i })
    await userEvent.click(chevron)

    expect(screen.getByText('child-a')).toBeInTheDocument()
    expect(screen.getByText('child-b')).toBeInTheDocument()
  })

  it('clicking the chevron again collapses child rows', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('group-one')).toBeInTheDocument()
    })

    const chevron = screen.getByRole('button', { name: /expand group-one/i })
    await userEvent.click(chevron)
    expect(screen.getByText('child-a')).toBeInTheDocument()

    await userEvent.click(chevron)
    expect(screen.queryByText('child-a')).not.toBeInTheDocument()
  })

  it('clicking the group name triggers drill-down (not chevron expand)', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('group-one')).toBeInTheDocument()
    })

    // Before any click: no chevron expand, children hidden
    expect(screen.queryByText('child-a')).not.toBeInTheDocument()

    // Click the group name span inside the table row (triggers onDrillDown → ?group=3)
    await userEvent.click(screen.getByText('group-one'))

    // Drill-down mode activated: children appear as flat top-level rows.
    await waitFor(() => {
      expect(screen.getByText('child-a')).toBeInTheDocument()
    })

    // In drill-down mode the chevron expand is NOT used — the group row itself is
    // gone from the table (group-one is now only in the breadcrumb h1, not a table row).
    // Verify: the expand chevron for group-one no longer appears (drill-down replaced it).
    expect(screen.queryByRole('button', { name: /expand group-one/i })).not.toBeInTheDocument()

    // The child rows shown via drill-down must NOT have the pl-8 indent class
    // (that class only appears on chevron-expanded children)
    const childCell = screen.getByText('child-a').closest('td')
    expect(childCell).not.toHaveClass('pl-8')
  })

  it('non-group (leaf) project row has no chevron button', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('proj-alpha')).toBeInTheDocument()
    })
    // Only group-one has a chevron; proj-alpha has none
    const chevrons = screen.queryAllByRole('button', { name: /expand/i })
    expect(chevrons).toHaveLength(1) // only group-one
  })

  it('group row uses Folder icon, leaf row uses FileText icon', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('group-one')).toBeInTheDocument()
    })
    // Icons are SVGs rendered by lucide-react — we detect them by their test-id
    // fallback: check aria labels or data attributes set by our implementation.
    // Since lucide doesn't add roles, we verify via testid set in DashboardProjectRow.
    expect(document.querySelector('[data-testid="icon-folder"]')).toBeInTheDocument()
    expect(document.querySelector('[data-testid="icon-file-text"]')).toBeInTheDocument()
  })
})

describe('Dashboard tree view — all mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: [] }),
    )
  })

  it('all mode shows no chevron buttons', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'All' })).toBeInTheDocument()
    })

    await userEvent.click(screen.getByRole('button', { name: 'All' }))

    await waitFor(() => {
      expect(screen.getByText('proj-alpha')).toBeInTheDocument()
    })

    const chevrons = screen.queryAllByRole('button', { name: /expand/i })
    expect(chevrons).toHaveLength(0)
  })
})
