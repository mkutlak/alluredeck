import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import * as dashboardApi from '@/api/dashboard'
import * as projectsApi from '@/api/projects'
import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/dashboard')
vi.mock('@/api/projects')
mockApiClient()
vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
}))
vi.mock('@/features/projects/CreateProjectDialog', () => ({ CreateProjectDialog: () => null }))

// Import AFTER mocks
import { DashboardPage } from '../DashboardPage'
import { useAuthStore, type AuthState, type Role } from '@/store/auth'
import type { DashboardData } from '@/types/api'

const mockData: DashboardData = {
  projects: [
    {
      project_id: 1,
      slug: 'proj-alpha',
      created_at: '2025-01-01T00:00:00Z',
      latest_build: {
        build_number: 5,
        created_at: '2025-03-01T10:00:00Z',
        statistics: { passed: 90, failed: 5, broken: 2, skipped: 3, unknown: 0, total: 100 },
        pass_rate: 90.0,
        duration_ms: 120000,
        flaky_count: 1,
        new_failed_count: 2,
        new_passed_count: 0,
      },
      sparkline: [],
    },
    {
      project_id: 2,
      slug: 'proj-beta',
      created_at: '2025-01-02T00:00:00Z',
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
  summary: { total_projects: 4, healthy: 1, degraded: 1, failing: 2 },
}

function renderPage(roles: Role[] = [], data: DashboardData = mockData) {
  vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
    (selector as (s: Partial<AuthState>) => unknown)({ roles }),
  )
  vi.mocked(dashboardApi.fetchDashboard).mockResolvedValue(data)
  return renderWithProviders(<DashboardPage />)
}

const expandButtons = () => screen.queryAllByRole('button', { name: /^expand/i })

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows group type and aggregate pass rate', async () => {
    renderPage()
    const groupRow = within((await screen.findByText('group-one')).closest('tr')!)
    expect(groupRow.getByText('Group')).toBeInTheDocument()
    expect(groupRow.getByText('87.8%')).toBeInTheDocument()
  })

  // Links use the numeric project_id, never the slug (9e053b3), and the pass rate
  // excludes skipped tests: 90 passed / (100 - 3 skipped) = 92.7%, not 90%.
  it('links a project by numeric id and excludes skipped tests from its pass rate', async () => {
    renderPage()
    const link = await screen.findByRole('link', { name: 'proj-alpha' })
    expect(link).toHaveAttribute('href', '/projects/1')
    expect(within(link.closest('tr')!).getByText('92.7%')).toBeInTheDocument()
  })

  it('shows empty state when no projects', async () => {
    renderPage([], {
      projects: [],
      summary: { total_projects: 0, healthy: 0, degraded: 0, failing: 0 },
    })
    expect(await screen.findByText(/no projects yet/i)).toBeInTheDocument()
  })

  it.each<[string, Role[], boolean]>([
    ['shows New project button for admin users', ['admin'], true],
    ['hides New project button for non-admin users', [], false],
  ])('%s', async (_name, roles, shown) => {
    renderPage(roles)
    await screen.findByText('proj-alpha')
    expect(!!screen.queryByRole('button', { name: /new project/i })).toBe(shown)
  })

  it('expands and collapses a group from its chevron, leaving leaf rows without one', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('group-one')
    expect(expandButtons()).toHaveLength(1)
    expect(screen.queryByText('child-a')).not.toBeInTheDocument()

    const chevron = screen.getByRole('button', { name: /expand group-one/i })
    await user.click(chevron)
    expect(screen.getByText('child-a')).toBeInTheDocument()
    expect(screen.getByText('child-b')).toBeInTheDocument()

    await user.click(chevron)
    expect(screen.queryByText('child-a')).not.toBeInTheDocument()
  })

  it('clicking the group name triggers drill-down (not chevron expand)', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByText('group-one'))

    // Drill-down lists only the group's children; inline expand would also
    // reveal child-a but keep the other top-level projects.
    expect(await screen.findByText('child-a')).toBeInTheDocument()
    expect(screen.queryByText('proj-alpha')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /expand group-one/i })).not.toBeInTheDocument()
  })

  it('switches to the flat "All" view: children as rows, no groups or chevrons', async () => {
    const user = userEvent.setup()
    renderPage()
    const grouped = await screen.findByRole('button', { name: 'Grouped' })
    const all = screen.getByRole('button', { name: 'All' })
    expect(grouped).toHaveAttribute('aria-pressed', 'true')

    await user.click(all)

    expect(all).toHaveAttribute('aria-pressed', 'true')
    expect(grouped).toHaveAttribute('aria-pressed', 'false')
    expect(await screen.findByText('child-a')).toBeInTheDocument()
    expect(screen.getByText('proj-alpha')).toBeInTheDocument()
    expect(screen.queryByText('group-one')).not.toBeInTheDocument()
    expect(expandButtons()).toHaveLength(0)
  })
  // Child slugs are unique only per parent, and the API resolves a slug top-level
  // first, then lowest id, so the dialogs must be handed the numeric id.
  describe('actions on a child whose slug another group also uses', () => {
    const child = (id: number, parentId: number) => ({
      project_id: id,
      slug: 'e2e',
      created_at: '2025-01-03T00:00:00Z',
      parent_id: parentId,
      latest_build: null,
      sparkline: [],
    })
    const group = (id: number, slug: string, childId: number) => ({
      project_id: id,
      slug,
      created_at: '2025-01-03T00:00:00Z',
      latest_build: null,
      sparkline: [],
      is_group: true,
      children: [child(childId, id)],
    })
    const twoGroups: DashboardData = {
      projects: [group(1, 'group-a', 5), group(2, 'group-b', 9)],
      summary: { total_projects: 4, healthy: 0, degraded: 0, failing: 0 },
    }

    async function openActions(user: ReturnType<typeof userEvent.setup>, item: RegExp) {
      renderPage(['admin'], twoGroups)
      await user.click(await screen.findByRole('button', { name: /expand group-b/i }))
      const row = screen.getByRole('link', { name: 'group-b/e2e' }).closest('tr')!
      await user.click(within(row).getByRole('button', { name: 'Project actions' }))
      await user.click(await screen.findByRole('menuitem', { name: item }))
    }

    it('deletes the clicked child by numeric id once its full label is typed', async () => {
      const user = userEvent.setup()
      await openActions(user, /delete project/i)

      const confirm = await screen.findByLabelText(/to confirm/i)
      await user.type(confirm, 'e2e')
      expect(screen.getByRole('button', { name: 'Delete project' })).toBeDisabled()
      await user.clear(confirm)
      await user.type(confirm, 'group-b/e2e')
      await user.click(screen.getByRole('button', { name: 'Delete project' }))

      await waitFor(() => expect(projectsApi.deleteProject).toHaveBeenCalledWith('9'))
    })

    it('renames the clicked child by numeric id', async () => {
      const user = userEvent.setup()
      await openActions(user, /rename project/i)

      await user.type(await screen.findByLabelText('New project ID'), 'smoke')
      await user.click(screen.getByRole('button', { name: 'Rename' }))

      await waitFor(() => expect(projectsApi.renameProject).toHaveBeenCalledWith('9', 'smoke'))
    })

    it('refuses to rename the clicked child to its current name', async () => {
      const user = userEvent.setup()
      await openActions(user, /rename project/i)

      await user.type(await screen.findByLabelText('New project ID'), 'e2e')
      await user.click(screen.getByRole('button', { name: 'Rename' }))

      expect(screen.getByText('New ID must differ from current')).toBeInTheDocument()
      expect(projectsApi.renameProject).not.toHaveBeenCalled()
    })

    it('moves the clicked child by numeric id and never offers it as its own parent', async () => {
      vi.mocked(projectsApi.getProjects).mockResolvedValue({
        data: [
          { project_id: 1, slug: 'group-a' },
          { project_id: 2, slug: 'group-b' },
          { project_id: 9, slug: 'e2e', parent_id: 2 },
        ],
        metadata: { message: 'ok' },
      } as Awaited<ReturnType<typeof projectsApi.getProjects>>)
      const user = userEvent.setup()
      await openActions(user, /move to group/i)

      await user.click(await screen.findByRole('combobox'))
      expect(screen.queryByRole('option', { name: 'e2e' })).not.toBeInTheDocument()
      await user.click(await screen.findByRole('option', { name: 'group-a' }))
      await user.click(screen.getByRole('button', { name: 'Move' }))

      await waitFor(() => expect(projectsApi.setProjectParent).toHaveBeenCalledWith('9', '1'))
    })
  })
})
