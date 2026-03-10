import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { useAuthStore } from '@/store/auth'
import type { AuthState, Role } from '@/store/auth'
import { ActionBar } from '../ActionBar'

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
}))

vi.mock('@/features/reports/SendResultsDialog', () => ({
  SendResultsDialog: vi.fn(({ open }: { open: boolean }) =>
    open ? <div data-testid="send-dialog">SendDialog</div> : null,
  ),
}))

vi.mock('@/features/reports/GenerateReportDialog', () => ({
  GenerateReportDialog: vi.fn(({ open }: { open: boolean }) =>
    open ? <div data-testid="generate-dialog">GenerateDialog</div> : null,
  ),
}))

vi.mock('@/features/reports/CleanDialog', () => ({
  CleanDialog: vi.fn(({ open, mode }: { open: boolean; mode: string }) =>
    open ? <div data-testid={`clean-dialog-${mode}`}>CleanDialog</div> : null,
  ),
}))

function renderActionBar(path: string, isAdminResult = true) {
  vi.mocked(useAuthStore).mockImplementation((selector: (state: AuthState) => unknown) =>
    selector({
      isAuthenticated: false,
      roles: isAdminResult ? (['admin'] as Role[]) : [],
      username: null,
      expiresAt: null,
      setAuth: vi.fn(),
      clearAuth: vi.fn(),
      isAdmin: () => isAdminResult,
      isSessionValid: () => true,
    }),
  )
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/" element={<ActionBar />} />
        <Route path="/projects/:id" element={<ActionBar />} />
        <Route path="/projects/:id/*" element={<ActionBar />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('ActionBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when no project is in the URL', () => {
    const { container } = renderActionBar('/')
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when user is not admin', () => {
    const { container } = renderActionBar('/projects/my-project', false)
    expect(container.firstChild).toBeNull()
  })

  it('renders all action buttons when projectId is present and user is admin', () => {
    renderActionBar('/projects/my-project')
    expect(screen.getByRole('button', { name: /send results/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /generate report/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /clean results/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /clean history/i })).toBeInTheDocument()
  })

  it('opens SendResultsDialog when "Send results" button is clicked', async () => {
    const user = userEvent.setup()
    renderActionBar('/projects/my-project')
    expect(screen.queryByTestId('send-dialog')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /send results/i }))
    expect(screen.getByTestId('send-dialog')).toBeInTheDocument()
  })

  it('opens GenerateReportDialog when "Generate report" button is clicked', async () => {
    const user = userEvent.setup()
    renderActionBar('/projects/my-project')
    expect(screen.queryByTestId('generate-dialog')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /generate report/i }))
    expect(screen.getByTestId('generate-dialog')).toBeInTheDocument()
  })

  it('opens CleanDialog with mode="results" when "Clean results" button is clicked', async () => {
    const user = userEvent.setup()
    renderActionBar('/projects/my-project')
    expect(screen.queryByTestId('clean-dialog-results')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /clean results/i }))
    expect(screen.getByTestId('clean-dialog-results')).toBeInTheDocument()
  })

  it('opens CleanDialog with mode="history" when "Clean history" button is clicked', async () => {
    const user = userEvent.setup()
    renderActionBar('/projects/my-project')
    expect(screen.queryByTestId('clean-dialog-history')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /clean history/i }))
    expect(screen.getByTestId('clean-dialog-history')).toBeInTheDocument()
  })
})
