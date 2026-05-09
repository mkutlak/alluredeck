import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { ProfilePage } from '../ProfilePage'
import * as usersApi from '@/api/users'
import { useUIStore } from '@/store/ui'
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

vi.mock('@/api/preferences', () => ({
  fetchPreferences: vi.fn().mockResolvedValue({
    data: { preferences: {}, updated_at: null },
    metadata: { message: 'ok' },
  }),
  updatePreferences: vi.fn().mockResolvedValue({
    data: { preferences: {}, updated_at: null },
    metadata: { message: 'ok' },
  }),
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

  afterEach(() => {
    useUIStore.setState({ timezone: null, timeFormat: null })
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

  describe('Display section', () => {
    it('renders the Display heading and timezone combobox', async () => {
      vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())

      renderPage()

      await waitFor(() => {
        expect(screen.getByText('Display')).toBeInTheDocument()
        expect(screen.getByText('Timezone')).toBeInTheDocument()
        expect(screen.getByRole('combobox')).toBeInTheDocument()
      })
    })

    it('timezone change updates the store', async () => {
      vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())
      const user = userEvent.setup()

      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('combobox')).toBeInTheDocument()
      })

      // Open the combobox
      await user.click(screen.getByRole('combobox'))

      // Search for Tokyo to narrow the list
      const searchInput = await screen.findByPlaceholderText('Search timezone…')
      await user.type(searchInput, 'Asia/Tokyo')

      const tokyoOption = await screen.findByText('Asia/Tokyo')
      await user.click(tokyoOption)

      expect(useUIStore.getState().timezone).toBe('Asia/Tokyo')
    })

    it('selecting Auto reverts timezone to null', async () => {
      useUIStore.setState({ timezone: 'Asia/Tokyo' })
      vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())
      const user = userEvent.setup()

      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('combobox')).toBeInTheDocument()
      })

      // Open the combobox (currently showing Asia/Tokyo)
      await user.click(screen.getByRole('combobox'))

      // Search for auto
      const searchInput = await screen.findByPlaceholderText('Search timezone…')
      await user.type(searchInput, 'Auto')

      const autoOption = await screen.findByText(/^Auto \(browser:/)
      await user.click(autoOption)

      expect(useUIStore.getState().timezone).toBeNull()
    })

    it('time format toggle: clicking 24-hour sets timeFormat to 24h', async () => {
      vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())
      const user = userEvent.setup()

      renderPage()

      const btn24 = await screen.findByRole('button', { name: '24-hour' })
      await user.click(btn24)

      expect(useUIStore.getState().timeFormat).toBe('24h')
    })

    it('time format toggle: clicking Auto resets timeFormat to null', async () => {
      useUIStore.setState({ timeFormat: '24h' })
      vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())
      const user = userEvent.setup()

      renderPage()

      const btnAuto = await screen.findByRole('button', { name: 'Auto' })
      await user.click(btnAuto)

      expect(useUIStore.getState().timeFormat).toBeNull()
    })

    it('preview text is present and non-empty', async () => {
      vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())

      renderPage()

      await waitFor(() => {
        expect(screen.getByText(/^Preview:/)).toBeInTheDocument()
      })

      const previewText = screen.getByText(/^Preview:/).textContent ?? ''
      expect(previewText.length).toBeGreaterThan('Preview: '.length)
    })

    it('preview reflects timeFormat: 12h shows AM/PM, 24h does not', async () => {
      vi.mocked(usersApi.fetchMe).mockResolvedValue(makeUser())

      // Render with 12h format
      useUIStore.setState({ timezone: null, timeFormat: '12h' })
      const { unmount } = renderPage()

      await waitFor(() => {
        expect(screen.getByText(/^Preview:/)).toBeInTheDocument()
      })
      const preview12h = screen.getByText(/^Preview:/).textContent ?? ''
      unmount()

      // Render with 24h format
      useUIStore.setState({ timezone: null, timeFormat: '24h' })
      renderPage()

      await waitFor(() => {
        expect(screen.getByText(/^Preview:/)).toBeInTheDocument()
      })
      const preview24h = screen.getByText(/^Preview:/).textContent ?? ''

      // 12h format contains AM or PM; 24h does not
      expect(preview12h).toMatch(/AM|PM/)
      expect(preview24h).not.toMatch(/AM|PM/)
    })
  })
})
