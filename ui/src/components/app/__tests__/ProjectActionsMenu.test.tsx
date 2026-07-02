import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useAuthStore } from '@/store/auth'
import type { AuthState, Role } from '@/store/auth'
import { ProjectActionsMenu } from '../ProjectActionsMenu'

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
  selectIsEditor: (s: { roles?: string[] }) =>
    (s.roles ?? []).includes('admin') || (s.roles ?? []).includes('editor'),
}))

vi.mock('@/lib/queries/projects', () => ({
  projectIndexOptions: () => ({
    queryKey: ['projects', 'index'],
    queryFn: () => Promise.resolve([{ project_id: 'my-project', report_type: 'allure' }]),
  }),
}))

vi.mock('@tanstack/react-query', async () => {
  const actual =
    await vi.importActual<typeof import('@tanstack/react-query')>('@tanstack/react-query')
  return {
    ...actual,
    useQuery: () => ({
      data: [{ project_id: 'my-project', report_type: 'allure' }],
      isLoading: false,
      error: null,
    }),
  }
})

vi.mock('@/features/reports/SendResultsDialog', () => ({
  SendResultsDialog: vi.fn(({ open }: { open: boolean }) =>
    open ? <div data-testid="send-dialog">SendDialog</div> : null,
  ),
}))

vi.mock('@/features/reports/CleanDialog', () => ({
  CleanDialog: vi.fn(({ open, mode }: { open: boolean; mode: string }) =>
    open ? <div data-testid={`clean-dialog-${mode}`}>CleanDialog</div> : null,
  ),
}))

function renderMenu(path: string, roles: Role[] = ['admin']) {
  vi.mocked(useAuthStore).mockImplementation((selector: (state: AuthState) => unknown) =>
    selector({
      isAuthenticated: false,
      roles,
      username: null,
      expiresAt: null,
      provider: null,
      setAuth: vi.fn(),
      clearAuth: vi.fn(),
    }),
  )
  return render(
    <TooltipProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/" element={<ProjectActionsMenu />} />
          <Route path="/projects/:id" element={<ProjectActionsMenu />} />
          <Route path="/projects/:id/*" element={<ProjectActionsMenu />} />
        </Routes>
      </MemoryRouter>
    </TooltipProvider>,
  )
}

describe('ProjectActionsMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when no project is in the URL', () => {
    const { container } = renderMenu('/')
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when user is not an editor (viewer)', () => {
    const { container } = renderMenu('/projects/my-project', [])
    expect(container.firstChild).toBeNull()
  })

  it('renders a single trigger button', () => {
    renderMenu('/projects/my-project')
    expect(screen.getByRole('button', { name: /project actions/i })).toBeInTheDocument()
  })

  it('opens a menu with all actions when the trigger is clicked', async () => {
    const user = userEvent.setup()
    renderMenu('/projects/my-project')
    await user.click(screen.getByRole('button', { name: /project actions/i }))
    expect(await screen.findByRole('menuitem', { name: /send results/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /clean results/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /clean history/i })).toBeInTheDocument()
  })

  it('opens SendResultsDialog when "Send results" menu item is clicked', async () => {
    const user = userEvent.setup()
    renderMenu('/projects/my-project')
    await user.click(screen.getByRole('button', { name: /project actions/i }))
    await user.click(await screen.findByRole('menuitem', { name: /send results/i }))
    expect(screen.getByTestId('send-dialog')).toBeInTheDocument()
  })

  it('opens CleanDialog with mode="results" when "Clean results" menu item is clicked', async () => {
    const user = userEvent.setup()
    renderMenu('/projects/my-project')
    await user.click(screen.getByRole('button', { name: /project actions/i }))
    await user.click(await screen.findByRole('menuitem', { name: /clean results/i }))
    expect(screen.getByTestId('clean-dialog-results')).toBeInTheDocument()
  })

  it('opens CleanDialog with mode="history" when "Clean history" menu item is clicked', async () => {
    const user = userEvent.setup()
    renderMenu('/projects/my-project')
    await user.click(screen.getByRole('button', { name: /project actions/i }))
    await user.click(await screen.findByRole('menuitem', { name: /clean history/i }))
    expect(screen.getByTestId('clean-dialog-history')).toBeInTheDocument()
  })

  it('styles destructive items ("Clean history") with text-destructive', async () => {
    const user = userEvent.setup()
    renderMenu('/projects/my-project')
    await user.click(screen.getByRole('button', { name: /project actions/i }))
    const item = await screen.findByRole('menuitem', { name: /clean history/i })
    expect(item.className).toContain('text-destructive')
  })

  it('shows "Send results" but not "Clean results"/"Clean history" for editors (admin-only actions)', async () => {
    const user = userEvent.setup()
    renderMenu('/projects/my-project', ['editor'])
    await user.click(screen.getByRole('button', { name: /project actions/i }))
    expect(await screen.findByRole('menuitem', { name: /send results/i })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: /clean results/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: /clean history/i })).not.toBeInTheDocument()
  })
})
