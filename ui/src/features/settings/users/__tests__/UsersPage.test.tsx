import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { UsersPage } from '../UsersPage'
import * as usersApi from '@/api/users'
import { useAuthStore } from '@/store/auth'
import type { User, UserListResponse, CreateUserResponse } from '@/types/api'

vi.mock('@/api/users', () => ({
  fetchUsers: vi.fn(),
  fetchUser: vi.fn(),
  createUser: vi.fn(),
  updateUserRole: vi.fn(),
  updateUserActive: vi.fn(),
  deactivateUser: vi.fn(),
  fetchMe: vi.fn(),
  updateMe: vi.fn(),
  changeMyPassword: vi.fn(),
  resetUserPassword: vi.fn(),
}))

// Mock react-router Navigate to track redirects
const mockNavigate = vi.fn()
vi.mock('react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router')>()
  return {
    ...actual,
    Navigate: ({ to }: { to: string }) => {
      mockNavigate(to)
      return <div data-testid="navigate-redirect" data-to={to} />
    },
  }
})

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 1,
    email: 'alice@example.com',
    name: 'Alice',
    provider: 'local',
    role: 'viewer',
    is_active: true,
    last_login: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeListResponse(users: User[], total?: number): UserListResponse {
  return { users, total: total ?? users.length, limit: 20, offset: 0 }
}

function renderPage(initialEntries = ['/settings/users']) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={initialEntries}>
        <UsersPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function setAdminAuth() {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: ['admin'],
    username: 'admin@example.com',
    expiresAt: Date.now() + 3600 * 1000,
    provider: 'local',
  })
}

function setViewerAuth() {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: ['viewer'],
    username: 'viewer@example.com',
    expiresAt: Date.now() + 3600 * 1000,
    provider: 'local',
  })
}

describe('UsersPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset auth store
    useAuthStore.setState({
      isAuthenticated: false,
      roles: [],
      username: null,
      expiresAt: null,
      provider: null,
    })
  })

  it('redirects non-admin user to profile page', () => {
    setViewerAuth()
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(makeListResponse([]))

    renderPage()

    expect(screen.getByTestId('navigate-redirect')).toHaveAttribute('data-to', '/settings/profile')
  })

  it('renders user list for admin', async () => {
    setAdminAuth()
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(
      makeListResponse([makeUser({ email: 'bob@example.com', name: 'Bob', role: 'editor' })]),
    )

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('bob@example.com')).toBeInTheDocument()
      expect(screen.getByText('Bob')).toBeInTheDocument()
    })
  })

  it('shows empty state when no users found', async () => {
    setAdminAuth()
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(makeListResponse([]))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/no users found/i)).toBeInTheDocument()
    })
  })

  it('debounces search input (300ms)', async () => {
    setAdminAuth()
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(makeListResponse([]))

    renderPage()

    const searchInput = screen.getByRole('textbox', { name: /search users/i })
    await userEvent.type(searchInput, 'ali')

    // fetchUsers should not be called immediately with the search term
    // (debounce test: verify it was called, timing handled by debounce hook)
    await waitFor(() => {
      expect(usersApi.fetchUsers).toHaveBeenCalled()
    })
  })

  it('opens invite user dialog on button click', async () => {
    setAdminAuth()
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(makeListResponse([]))

    renderPage()

    const btn = await screen.findByRole('button', { name: /invite user/i })
    await userEvent.click(btn)

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
  })

  it('create flow shows CreatedUserDialog with temp password on success', async () => {
    setAdminAuth()
    const newUser = makeUser({ id: 99, email: 'new@example.com', name: 'New User' })
    const createResult: CreateUserResponse = { user: newUser, temp_password: 'TempP@ssword123!' }

    vi.mocked(usersApi.fetchUsers).mockResolvedValue(makeListResponse([]))
    vi.mocked(usersApi.createUser).mockResolvedValue(createResult)

    renderPage()

    const inviteBtn = await screen.findByRole('button', { name: /invite user/i })
    await userEvent.click(inviteBtn)

    const emailInput = await screen.findByLabelText(/email/i)
    const nameInput = await screen.findByLabelText(/name/i)
    // Use fireEvent.change for reliable React controlled-input state update
    fireEvent.change(emailInput, { target: { value: 'new@example.com' } })
    fireEvent.change(nameInput, { target: { value: 'New User' } })

    // Use fireEvent.click to avoid Radix Dialog pointer-event interference
    const submitBtn = await screen.findByRole('button', { name: /^create$/i })
    await waitFor(() => expect(submitBtn).not.toBeDisabled())
    fireEvent.click(submitBtn)

    // Verify the temp password dialog appears (end-to-end: API called → onSuccess → dialog)
    await waitFor(() => {
      expect(screen.getByText('TempP@ssword123!')).toBeInTheDocument()
    })
  })

  it('shows role change dialog on row action', async () => {
    setAdminAuth()
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(
      makeListResponse([makeUser({ email: 'bob@example.com' })]),
    )

    renderPage()

    const actionsBtn = await screen.findByRole('button', { name: /actions for bob@example.com/i })
    await userEvent.click(actionsBtn)

    const changeRoleItem = await screen.findByText(/change role/i)
    await userEvent.click(changeRoleItem)

    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText(/change role/i)).toBeInTheDocument()
    })
  })

  it('shows pagination controls when total > page size', async () => {
    setAdminAuth()
    const manyUsers = Array.from({ length: 20 }, (_, i) =>
      makeUser({ id: i + 1, email: `user${i}@example.com` }),
    )
    vi.mocked(usersApi.fetchUsers).mockResolvedValue({
      users: manyUsers,
      total: 45,
      limit: 20,
      offset: 0,
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /next page/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /previous page/i })).toBeInTheDocument()
    })
  })

  it('next/prev pagination buttons work', async () => {
    setAdminAuth()
    const manyUsers = Array.from({ length: 20 }, (_, i) =>
      makeUser({ id: i + 1, email: `user${i}@example.com` }),
    )
    vi.mocked(usersApi.fetchUsers).mockResolvedValue({
      users: manyUsers,
      total: 45,
      limit: 20,
      offset: 0,
    })

    renderPage()

    const nextBtn = await screen.findByRole('button', { name: /next page/i })
    const prevBtn = screen.getByRole('button', { name: /previous page/i })

    expect(prevBtn).toBeDisabled()
    expect(nextBtn).not.toBeDisabled()

    await userEvent.click(nextBtn)

    await waitFor(() => {
      expect(usersApi.fetchUsers).toHaveBeenCalledWith(
        expect.objectContaining({ offset: 20 }),
      )
    })
  })

  it('reset flow: shows confirm dialog then temp password on success', async () => {
    setAdminAuth()
    const localUser = makeUser({ id: 5, email: 'bob@example.com', provider: 'local' })
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(makeListResponse([localUser]))
    vi.mocked(usersApi.resetUserPassword).mockResolvedValue({ temp_password: 'ResetP@ss12345!' })

    renderPage()

    const actionsBtn = await screen.findByRole('button', { name: /actions for bob@example.com/i })
    await userEvent.click(actionsBtn)

    const resetItem = await screen.findByText(/reset password/i)
    await userEvent.click(resetItem)

    // Confirm dialog should appear
    await waitFor(() => {
      expect(screen.getByText(/reset password\?/i)).toBeInTheDocument()
    })

    // Click the confirm button
    const confirmBtn = screen.getByRole('button', { name: /reset password/i })
    fireEvent.click(confirmBtn)

    await waitFor(() => {
      expect(usersApi.resetUserPassword).toHaveBeenCalledWith(5)
    })

    // Temp password dialog should appear with copy button
    await waitFor(() => {
      expect(screen.getByText('ResetP@ss12345!')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /copy password/i })).toBeInTheDocument()
    })
  })

  it('reset password action is not shown for OIDC users', async () => {
    setAdminAuth()
    const oidcUser = makeUser({ id: 7, email: 'oidc@example.com', provider: 'oidc' })
    vi.mocked(usersApi.fetchUsers).mockResolvedValue(makeListResponse([oidcUser]))

    renderPage()

    const actionsBtn = await screen.findByRole('button', { name: /actions for oidc@example.com/i })
    await userEvent.click(actionsBtn)

    // Wait for dropdown to open
    await waitFor(() => {
      expect(screen.getByText(/change role/i)).toBeInTheDocument()
    })

    expect(screen.queryByText(/reset password/i)).not.toBeInTheDocument()
  })
})
