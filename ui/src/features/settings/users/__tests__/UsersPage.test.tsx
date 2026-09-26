import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { UsersPage } from '../UsersPage'
import * as usersApi from '@/api/users'
import { useAuthStore, type Role } from '@/store/auth'
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

function renderPage(list: UserListResponse, roles: Role[] = ['admin']) {
  useAuthStore.setState({
    isAuthenticated: true,
    roles,
    username: `${roles[0]}@example.com`,
    expiresAt: Date.now() + 3600 * 1000,
    provider: 'local',
  })
  vi.mocked(usersApi.fetchUsers).mockResolvedValue(list)
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/settings/users']}>
        <Routes>
          <Route path="/settings/users" element={<UsersPage />} />
          <Route path="/settings/profile" element={<p>profile page</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

async function openActions(email: string) {
  await userEvent.click(
    await screen.findByRole('button', { name: new RegExp(`actions for ${email}`, 'i') }),
  )
}

describe('UsersPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('redirects non-admin user to profile page', () => {
    renderPage(makeListResponse([]), ['viewer'])
    expect(screen.getByText('profile page')).toBeInTheDocument()
    expect(usersApi.fetchUsers).not.toHaveBeenCalled()
  })

  it.each([
    {
      name: 'users',
      users: [makeUser({ email: 'bob@example.com', name: 'Bob', role: 'editor' })],
      texts: ['bob@example.com', 'Bob'],
    },
    { name: 'the empty state', users: [], texts: [/no users found/i] },
  ])('lists $name for an admin', async ({ users, texts }) => {
    renderPage(makeListResponse(users))
    for (const text of texts) expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it('create flow shows CreatedUserDialog with temp password on success', async () => {
    const newUser = makeUser({ id: 99, email: 'new@example.com', name: 'New User' })
    const createResult: CreateUserResponse = { user: newUser, temp_password: 'TempP@ssword123!' }
    vi.mocked(usersApi.createUser).mockResolvedValue(createResult)
    renderPage(makeListResponse([]))

    await userEvent.click(await screen.findByRole('button', { name: /invite user/i }))
    // fireEvent keeps the controlled inputs and the submit clear of Radix Dialog pointer handling.
    fireEvent.change(await screen.findByLabelText(/email/i), {
      target: { value: 'new@example.com' },
    })
    fireEvent.change(await screen.findByLabelText(/name/i), { target: { value: 'New User' } })
    const submitBtn = await screen.findByRole('button', { name: /^create$/i })
    await waitFor(() => expect(submitBtn).not.toBeDisabled())
    fireEvent.click(submitBtn)

    // End-to-end: API called → onSuccess → temp password dialog.
    expect(await screen.findByText('TempP@ssword123!')).toBeInTheDocument()
  })

  it('pages with next/prev when total exceeds the page size', async () => {
    const manyUsers = Array.from({ length: 20 }, (_, i) =>
      makeUser({ id: i + 1, email: `user${i}@example.com` }),
    )
    renderPage(makeListResponse(manyUsers, 45))

    const nextBtn = await screen.findByRole('button', { name: /next page/i })
    expect(screen.getByRole('button', { name: /previous page/i })).toBeDisabled()
    expect(nextBtn).not.toBeDisabled()

    await userEvent.click(nextBtn)
    await waitFor(() => {
      expect(usersApi.fetchUsers).toHaveBeenCalledWith(expect.objectContaining({ offset: 20 }))
    })
  })

  it('reset flow: shows confirm dialog then temp password on success', async () => {
    vi.mocked(usersApi.resetUserPassword).mockResolvedValue({ temp_password: 'ResetP@ss12345!' })
    renderPage(makeListResponse([makeUser({ id: 5, email: 'bob@example.com', provider: 'local' })]))

    await openActions('bob@example.com')
    await userEvent.click(await screen.findByText(/reset password/i))
    expect(await screen.findByText(/reset password\?/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /reset password/i }))

    await waitFor(() => {
      expect(usersApi.resetUserPassword).toHaveBeenCalledWith(5)
    })
    expect(await screen.findByText('ResetP@ss12345!')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /copy password/i })).toBeInTheDocument()
  })

  it('offers no password reset for OIDC users, and opens the role dialog from the row action', async () => {
    renderPage(makeListResponse([makeUser({ id: 7, email: 'oidc@example.com', provider: 'oidc' })]))

    await openActions('oidc@example.com')
    const changeRole = await screen.findByText(/change role/i)
    expect(screen.queryByText(/reset password/i)).not.toBeInTheDocument()

    await userEvent.click(changeRole)
    expect(await screen.findByRole('dialog')).toHaveTextContent(/change role/i)
  })
})
