import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useAuthStore } from '@/store/auth'
import type { AuthState, Role } from '@/store/auth'
import { CreateMenu } from '../CreateMenu'

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
}))

vi.mock('@/features/projects/CreateProjectDialog', () => ({
  CreateProjectDialog: vi.fn(({ open }: { open: boolean }) =>
    open ? <div data-testid="create-project-dialog">Dialog</div> : null,
  ),
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

function mockAdmin(isAdminResult: boolean) {
  vi.mocked(useAuthStore).mockImplementation((selector: (state: AuthState) => unknown) =>
    selector({
      isAuthenticated: false,
      roles: isAdminResult ? (['admin'] as Role[]) : [],
      username: null,
      expiresAt: null,
      provider: null,
      setAuth: vi.fn(),
      clearAuth: vi.fn(),
    }),
  )
}

describe('CreateMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns null when user is not admin', () => {
    mockAdmin(false)
    const { container } = render(<CreateMenu />)
    expect(container.firstChild).toBeNull()
  })

  it('renders "+" button when user is admin', () => {
    mockAdmin(true)
    render(<CreateMenu />)
    expect(screen.getByRole('button', { name: /create new/i })).toBeInTheDocument()
  })

  it('clicking "+" opens a dropdown menu', async () => {
    const user = userEvent.setup()
    mockAdmin(true)
    render(<CreateMenu />)
    const button = screen.getByRole('button', { name: /create new/i })
    await user.click(button)
    expect(screen.getByRole('menuitem', { name: /new project/i })).toBeInTheDocument()
  })

  it('dropdown contains "New Project" item', async () => {
    const user = userEvent.setup()
    mockAdmin(true)
    render(<CreateMenu />)
    await user.click(screen.getByRole('button', { name: /create new/i }))
    expect(screen.getByRole('menuitem', { name: /new project/i })).toBeInTheDocument()
  })

  it('clicking "New Project" opens the CreateProjectDialog', async () => {
    const user = userEvent.setup()
    mockAdmin(true)
    render(<CreateMenu />)
    await user.click(screen.getByRole('button', { name: /create new/i }))
    await user.click(screen.getByRole('menuitem', { name: /new project/i }))
    expect(screen.getByTestId('create-project-dialog')).toBeInTheDocument()
  })
})
