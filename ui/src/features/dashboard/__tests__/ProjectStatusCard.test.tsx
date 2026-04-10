import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import type { DashboardProjectEntry } from '@/types/api'

vi.mock('recharts', () => ({
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  LineChart: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Line: () => null,
}))

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
}))
vi.mock('@/features/projects/DeleteProjectDialog', () => ({
  DeleteProjectDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="delete-dialog" /> : null,
}))
import { mockApiClient } from '@/test/mocks/api-client'
mockApiClient()

import { ProjectStatusCard } from '../ProjectStatusCard'
import { useAuthStore } from '@/store/auth'

import type { AuthState } from '@/store/auth'

type AuthSelector = (s: Partial<AuthState>) => unknown

function renderCard(project: DashboardProjectEntry) {
  return render(
    <MemoryRouter>
      <ProjectStatusCard project={project} />
    </MemoryRouter>,
  )
}

const healthyProject: DashboardProjectEntry = {
  project_id: 1,
  slug: 'my-project',
  created_at: '2025-01-01T00:00:00Z',
  latest_build: {
    build_order: 10,
    created_at: '2025-03-01T12:00:00Z',
    statistics: { passed: 95, failed: 2, broken: 1, skipped: 2, unknown: 0, total: 100 },
    pass_rate: 95.0,
    duration_ms: 60000,
    flaky_count: 0,
    new_failed_count: 0,
    new_passed_count: 1,
    ci_branch: 'main',
  },
  sparkline: [
    { build_order: 9, pass_rate: 90 },
    { build_order: 10, pass_rate: 95 },
  ],
}

const noBuildsProject: DashboardProjectEntry = {
  project_id: 2,
  slug: 'empty-project',
  created_at: '2025-01-01T00:00:00Z',
  latest_build: null,
  sparkline: [],
}

describe('ProjectStatusCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: non-admin user
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: [] }),
    )
  })

  it('displays project name', () => {
    renderCard(healthyProject)
    expect(screen.getByText('my-project')).toBeInTheDocument()
  })

  it('shows pass rate badge for healthy project', () => {
    renderCard(healthyProject)
    expect(screen.getByText('95%')).toBeInTheDocument()
  })

  it('shows "No builds" when latest_build is null', () => {
    renderCard(noBuildsProject)
    expect(screen.getByText(/no builds/i)).toBeInTheDocument()
  })

  it('shows CI branch when available', () => {
    renderCard(healthyProject)
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('renders a link to the project', () => {
    renderCard(healthyProject)
    const link = screen.getByRole('link', { name: 'my-project' })
    expect(link).toBeInTheDocument()
    expect(link.getAttribute('href')).toBe('/projects/my-project')
  })

  it('shows delete dropdown trigger for admin users', () => {
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: ['admin'] }),
    )
    renderCard(healthyProject)
    expect(screen.getByRole('button', { name: /project actions/i })).toBeInTheDocument()
  })

  it('hides delete dropdown for non-admin users', () => {
    renderCard(healthyProject)
    expect(screen.queryByRole('button', { name: /project actions/i })).not.toBeInTheDocument()
  })

  it('does not mount DeleteProjectDialog when deleteOpen is false', () => {
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: ['admin'] }),
    )
    renderCard(healthyProject)
    expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()
  })
})
