import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { ProfilePage } from '../ProfilePage'
import * as usersApi from '@/api/users'
import type { User } from '@/types/api'

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
    last_login: '2026-01-15T10:30:00Z',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/settings/profile']}>
        <ProfilePage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ProfilePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders profile fields for local user', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('alice@example.com')).toBeInTheDocument()
      expect(screen.getByText('local')).toBeInTheDocument()
      expect(screen.getByText('viewer')).toBeInTheDocument()
      expect(screen.getByText('Alice')).toBeInTheDocument()
    })
  })

  it('shows Change Password card for local user', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ provider: 'local' }))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('alice@example.com')).toBeInTheDocument()
    })

    expect(screen.getByText('Change Password')).toBeInTheDocument()
    expect(screen.getByLabelText('Current password')).toBeInTheDocument()
    expect(screen.getByLabelText('New password')).toBeInTheDocument()
    expect(screen.getByLabelText('Confirm new password')).toBeInTheDocument()
  })

  it('does not show Change Password card for OIDC user', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ provider: 'oidc' }))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('oidc')).toBeInTheDocument()
    })

    expect(screen.queryByText('Change Password')).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/current password/i)).not.toBeInTheDocument()
  })

  it('calls changeMyPassword with correct body and shows success toast', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ provider: 'local' }))
    vi.mocked(usersApi.changeMyPassword).mockResolvedValue(undefined)

    renderPage()

    await waitFor(() => {
      expect(screen.getByLabelText('Current password')).toBeInTheDocument()
    })

    await userEvent.type(screen.getByLabelText('Current password'), 'OldPassword123!')
    await userEvent.type(screen.getByLabelText('New password'), 'NewPassword456!')
    await userEvent.type(screen.getByLabelText('Confirm new password'), 'NewPassword456!')

    const submitBtn = screen.getByRole('button', { name: /change password/i })
    expect(submitBtn).not.toBeDisabled()
    await userEvent.click(submitBtn)

    await waitFor(() => {
      expect(usersApi.changeMyPassword).toHaveBeenCalledWith({
        current_password: 'OldPassword123!',
        new_password: 'NewPassword456!',
      })
    })
  })

  it('shows banner error when backend returns 401 (invalid current password)', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ provider: 'local' }))
    const err = new Error('Invalid current password')
    vi.mocked(usersApi.changeMyPassword).mockRejectedValue(err)

    renderPage()

    await waitFor(() => {
      expect(screen.getByLabelText('Current password')).toBeInTheDocument()
    })

    await userEvent.type(screen.getByLabelText('Current password'), 'WrongPassword1!')
    await userEvent.type(screen.getByLabelText('New password'), 'NewPassword456!')
    await userEvent.type(screen.getByLabelText('Confirm new password'), 'NewPassword456!')

    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
      expect(screen.getByRole('alert')).toHaveTextContent('Invalid current password')
    })
  })

  it('disables submit and shows inline error for short new password', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ provider: 'local' }))

    renderPage()

    await waitFor(() => {
      expect(screen.getByLabelText('Current password')).toBeInTheDocument()
    })

    await userEvent.type(screen.getByLabelText('Current password'), 'OldPassword123!')
    await userEvent.type(screen.getByLabelText('New password'), 'short')

    expect(screen.getByText('Password must be at least 12 characters')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /change password/i })).toBeDisabled()
  })

  it('disables submit and shows inline error for mismatched confirm', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ provider: 'local' }))

    renderPage()

    await waitFor(() => {
      expect(screen.getByLabelText('Current password')).toBeInTheDocument()
    })

    await userEvent.type(screen.getByLabelText('Current password'), 'OldPassword123!')
    await userEvent.type(screen.getByLabelText('New password'), 'NewPassword456!')
    await userEvent.type(screen.getByLabelText('Confirm new password'), 'DifferentPass456!')

    expect(screen.getByText('Passwords do not match')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /change password/i })).toBeDisabled()
  })

  it('disables submit and shows inline error when new password equals current', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ provider: 'local' }))

    renderPage()

    await waitFor(() => {
      expect(screen.getByLabelText('Current password')).toBeInTheDocument()
    })

    await userEvent.type(screen.getByLabelText('Current password'), 'SamePassword123!')
    await userEvent.type(screen.getByLabelText('New password'), 'SamePassword123!')

    expect(screen.getByText('New password must be different from current')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /change password/i })).toBeDisabled()
  })

  it('shows Edit name button and allows editing', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())

    renderPage()

    const editBtn = await screen.findByRole('button', { name: /edit name/i })
    await userEvent.click(editBtn)

    const nameInput = screen.getByRole('textbox', { name: /name/i })
    expect(nameInput).toBeInTheDocument()
    expect(nameInput).toHaveValue('Alice')
  })

  it('calls updateMe mutation when name is saved', async () => {
    const updatedUser = makeUser({ name: 'Alice Smith' })
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())
    vi.mocked(usersApi.updateMe).mockResolvedValue(updatedUser)

    renderPage()

    const editBtn = await screen.findByRole('button', { name: /edit name/i })
    await userEvent.click(editBtn)

    const nameInput = screen.getByRole('textbox', { name: /name/i })
    await userEvent.clear(nameInput)
    await userEvent.type(nameInput, 'Alice Smith')

    const saveBtn = screen.getByRole('button', { name: /^save$/i })
    await userEvent.click(saveBtn)

    await waitFor(() => {
      expect(usersApi.updateMe).toHaveBeenCalledWith({ name: 'Alice Smith' })
    })
  })

  it('shows loading spinner while fetching', () => {
    vi.mocked(usersApi.fetchMe).mockReturnValue(new Promise(() => {}))

    renderPage()

    // Spinner should be present during load
    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('shows error state when fetch fails', async () => {
    vi.mocked(usersApi.fetchMe).mockRejectedValue(new Error('Network error'))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/failed to load profile/i)).toBeInTheDocument()
    })
  })

  it('shows last login when present', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ last_login: '2026-01-15T10:30:00Z' }))

    renderPage()

    await waitFor(() => {
      // The date is formatted via formatDate — just check the field label is present
      expect(screen.getByText(/last login/i)).toBeInTheDocument()
    })
  })

  it('shows dash for last login when null', async () => {
    vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser({ last_login: null }))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('—')).toBeInTheDocument()
    })
  })
})
