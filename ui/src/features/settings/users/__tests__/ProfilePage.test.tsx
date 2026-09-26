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

function renderPage(me: User | Error = makeUser()) {
  if (me instanceof Error) vi.mocked(usersApi.fetchMe).mockRejectedValue(me)
  else vi.mocked(usersApi.fetchMe).mockResolvedValue(me)
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/settings/profile']}>
        <ProfilePage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

async function fillPasswords(current: string, next: string, confirm = '') {
  await userEvent.type(await screen.findByLabelText('Current password'), current)
  await userEvent.type(screen.getByLabelText('New password'), next)
  if (confirm) await userEvent.type(screen.getByLabelText('Confirm new password'), confirm)
}

const submit = () => screen.getByRole('button', { name: /change password/i })

describe('ProfilePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    useUIStore.setState({ timezone: null, timeFormat: null })
  })

  it.each([
    { provider: 'local' as const, lastLogin: '2026-01-15T10:30:00Z', card: true },
    { provider: 'oidc' as const, lastLogin: null, card: false },
  ])(
    'shows the Change Password card only for local users ($provider)',
    async ({ provider, lastLogin, card }) => {
      renderPage(makeUser({ provider, last_login: lastLogin }))

      expect(await screen.findByText(provider)).toBeInTheDocument()
      expect(screen.queryByText('Change Password') !== null).toBe(card)
      expect(screen.queryByLabelText('Current password') !== null).toBe(card)
      expect(screen.queryByLabelText('New password') !== null).toBe(card)
      expect(screen.queryByLabelText('Confirm new password') !== null).toBe(card)
      // A missing last login renders as a dash.
      expect(screen.queryByText('—') !== null).toBe(lastLogin === null)
    },
  )

  it('calls changeMyPassword with correct body', async () => {
    vi.mocked(usersApi.changeMyPassword).mockResolvedValue(undefined)
    renderPage()

    await fillPasswords('OldPassword123!', 'NewPassword456!', 'NewPassword456!')
    expect(submit()).not.toBeDisabled()
    await userEvent.click(submit())

    await waitFor(() => {
      expect(usersApi.changeMyPassword).toHaveBeenCalledWith({
        current_password: 'OldPassword123!',
        new_password: 'NewPassword456!',
      })
    })
  })

  it('shows banner error when backend returns 401 (invalid current password)', async () => {
    vi.mocked(usersApi.changeMyPassword).mockRejectedValue(new Error('Invalid current password'))
    renderPage()

    await fillPasswords('WrongPassword1!', 'NewPassword456!', 'NewPassword456!')
    await userEvent.click(submit())

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid current password')
  })

  // Confirm matches the new password unless given, so each row trips exactly one rule.
  it.each([
    { rule: 'too short', next: 'short', error: 'Password must be at least 12 characters' },
    {
      rule: 'confirm mismatch',
      next: 'NewPassword456!',
      confirm: 'DifferentPass456!',
      error: 'Passwords do not match',
    },
    {
      rule: 'same as current',
      current: 'SamePassword123!',
      next: 'SamePassword123!',
      error: 'New password must be different from current',
    },
  ])(
    'disables submit and shows an inline error: $rule',
    async ({ current = 'OldPassword123!', next, confirm = next, error }) => {
      renderPage()

      await fillPasswords(current, next, confirm)
      expect(screen.getByText(error)).toBeInTheDocument()
      expect(submit()).toBeDisabled()
    },
  )

  it('edits the name and saves it via updateMe', async () => {
    vi.mocked(usersApi.updateMe).mockResolvedValue(makeUser({ name: 'Alice Smith' }))
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: /edit name/i }))
    const nameInput = screen.getByRole('textbox', { name: /name/i })
    expect(nameInput).toHaveValue('Alice')
    await userEvent.clear(nameInput)
    await userEvent.type(nameInput, 'Alice Smith')
    await userEvent.click(screen.getByRole('button', { name: /^save$/i }))

    await waitFor(() => {
      expect(usersApi.updateMe).toHaveBeenCalledWith({ name: 'Alice Smith' })
    })
  })

  it('shows error state when fetch fails', async () => {
    renderPage(new Error('Network error'))

    expect(await screen.findByText(/failed to load profile/i)).toBeInTheDocument()
  })

  describe('Display section', () => {
    it.each([
      { initial: null, search: 'Asia/Tokyo', option: 'Asia/Tokyo', want: 'Asia/Tokyo' },
      { initial: 'Asia/Tokyo', search: 'Auto', option: /^Auto \(browser:/, want: null },
    ])('picking timezone $option stores $want', async ({ initial, search, option, want }) => {
      useUIStore.setState({ timezone: initial })
      const user = userEvent.setup()
      renderPage()

      await user.click(await screen.findByRole('combobox'))
      await user.type(await screen.findByPlaceholderText('Search timezone…'), search)
      await user.click(await screen.findByText(option))

      expect(useUIStore.getState().timezone).toBe(want)
    })

    // The preview renders the current time in the chosen format.
    it.each([
      { initial: null, click: '24-hour', want: '24h', ampm: false },
      { initial: null, click: '12-hour', want: '12h', ampm: true },
      { initial: '24h' as const, click: 'Auto', want: null, ampm: undefined },
    ])('time format $click sets $want', async ({ initial, click, want, ampm }) => {
      useUIStore.setState({ timeFormat: initial })
      const user = userEvent.setup()
      renderPage()

      await user.click(await screen.findByRole('button', { name: click }))

      expect(useUIStore.getState().timeFormat).toBe(want)
      if (ampm !== undefined) {
        expect(/AM|PM/.test(screen.getByText(/^Preview:/).textContent ?? '')).toBe(ampm)
      }
    })
  })
})
