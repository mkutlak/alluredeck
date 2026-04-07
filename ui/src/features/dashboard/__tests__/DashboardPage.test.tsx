import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
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

function renderPage() {
  return renderWithProviders(<DashboardPage />)
}

// Import AFTER mocks
import { DashboardPage } from '../DashboardPage'
import { useAuthStore } from '@/store/auth'
import type { DashboardData } from '@/types/api'

const mockData: DashboardData = {
  projects: [
    {
      project_id: 'proj-alpha',
      created_at: '2025-01-01T00:00:00Z',
      latest_build: {
        build_order: 5,
        created_at: '2025-03-01T10:00:00Z',
        statistics: { passed: 90, failed: 5, broken: 2, skipped: 3, unknown: 0, total: 100 },
        pass_rate: 90.0,
        duration_ms: 120000,
        flaky_count: 1,
        new_failed_count: 2,
        new_passed_count: 0,
      },
      sparkline: [
        { build_order: 3, pass_rate: 85 },
        { build_order: 4, pass_rate: 88 },
        { build_order: 5, pass_rate: 90 },
      ],
    },
    {
      project_id: 'proj-beta',
      created_at: '2025-01-02T00:00:00Z',
      latest_build: null,
      sparkline: [],
    },
    {
      project_id: 'group-one',
      created_at: '2025-01-03T00:00:00Z',
      latest_build: null,
      sparkline: [],
      is_group: true,
      aggregate: { passed: 72, failed: 8, broken: 2, skipped: 0, total: 82, pass_rate: 87.8 },
      children: [
        {
          project_id: 'child-a',
          created_at: '2025-01-03T00:00:00Z',
          latest_build: {
            build_order: 1,
            created_at: '2025-03-01T10:00:00Z',
            statistics: { passed: 40, failed: 2, broken: 0, skipped: 0, unknown: 0, total: 42 },
            pass_rate: 95.2,
            duration_ms: 60000,
            flaky_count: 0,
            new_failed_count: 0,
            new_passed_count: 0,
          },
          sparkline: [],
        },
        {
          project_id: 'child-b',
          created_at: '2025-01-03T00:00:00Z',
          latest_build: null,
          sparkline: [],
        },
      ],
    },
  ],
  summary: { total_projects: 4, healthy: 1, degraded: 1, failing: 2 },
}

import type { AuthState } from '@/store/auth'

type AuthSelector = (s: Partial<AuthState>) => unknown

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: [] }),
    )
  })

  it('renders loading state initially', () => {
    vi.mocked(dashboardApi.fetchDashboard).mockReturnValue(new Promise(() => {}))
    renderPage()
    const skeletons = document.querySelectorAll('[class*="animate-pulse"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('renders table with column headers', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('Name')).toBeInTheDocument()
      expect(screen.getByText('Type')).toBeInTheDocument()
      expect(screen.getByText('Pass Rate')).toBeInTheDocument()
    })
  })

  it('renders project names in table rows', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('proj-alpha')).toBeInTheDocument()
      expect(screen.getByText('proj-beta')).toBeInTheDocument()
      expect(screen.getByText('group-one')).toBeInTheDocument()
    })
  })

  it('shows group type and aggregate pass rate', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('group-one')).toBeInTheDocument()
      expect(screen.getByText('88%')).toBeInTheDocument()
      expect(screen.getByText('Group')).toBeInTheDocument()
    })
  })

  it('shows empty state when no projects', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue({
      projects: [],
      summary: { total_projects: 0, healthy: 0, degraded: 0, failing: 0 },
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/no projects/i)).toBeInTheDocument()
    })
  })

  it("shows 'Projects' heading", async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /projects/i })).toBeInTheDocument()
    })
  })

  it('shows New project button for admin users', async () => {
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: ['admin'] }),
    )
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /new project/i })).toBeInTheDocument()
    })
  })

  it('hides New project button for non-admin users', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('proj-alpha')).toBeInTheDocument()
    })
    expect(screen.queryByRole('button', { name: /new project/i })).not.toBeInTheDocument()
  })

  it('calls fetchDashboard with no arguments', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('proj-alpha')).toBeInTheDocument()
    })
    expect(vi.mocked(dashboardApi.fetchDashboard)).toHaveBeenCalledWith()
  })

  it('shows Grouped/All toggle', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Grouped' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'All' })).toBeInTheDocument()
    })
  })

  it('shows search input', async () => {
    vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(mockData)
    renderPage()
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search...')).toBeInTheDocument()
    })
  })
})
