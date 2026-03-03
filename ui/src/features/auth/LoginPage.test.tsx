import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { LoginPage } from './LoginPage'
import * as authApi from '@/api/auth'
import { useAuthStore } from '@/store/auth'

// Mock the auth API
vi.mock('@/api/auth')
// Mock the API client — no more setAccessToken/getAccessToken
vi.mock('@/api/client', () => ({
  apiClient: { post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

const mockNavigate = vi.fn()
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useNavigate: () => mockNavigate }
})

function renderLoginPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.setState({ isAuthenticated: false, roles: [], username: null, expiresAt: null })
  })

  it('renders username and password fields', () => {
    renderLoginPage()
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
  })

  it('shows error when fields are empty', async () => {
    const user = userEvent.setup()
    renderLoginPage()
    await user.click(screen.getByRole('button', { name: /sign in/i }))
    expect(screen.getByRole('alert')).toHaveTextContent(/required/i)
  })

  it('calls login API with credentials', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.login).mockResolvedValue({
      data: { csrf_token: 'csrf123', expires_in: 3600, roles: ['admin'] },
      metadata: { message: 'ok' },
    })

    renderLoginPage()
    await user.type(screen.getByLabelText(/username/i), 'admin')
    await user.type(screen.getByLabelText(/password/i), 'secret')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => {
      // TanStack Query v5 passes an internal context object as the second arg to mutationFn
      expect(authApi.login).toHaveBeenCalledWith(
        { username: 'admin', password: 'secret' },
        expect.anything(),
      )
    })
  })

  it('navigates to / on successful login', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.login).mockResolvedValue({
      data: { csrf_token: 'csrf123', expires_in: 3600, roles: ['admin'] },
      metadata: { message: 'ok' },
    })

    renderLoginPage()
    await user.type(screen.getByLabelText(/username/i), 'admin')
    await user.type(screen.getByLabelText(/password/i), 'secret')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true })
    })
  })

  it('displays API error message on failure', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.login).mockRejectedValue(new Error('Invalid username/password'))

    renderLoginPage()
    await user.type(screen.getByLabelText(/username/i), 'admin')
    await user.type(screen.getByLabelText(/password/i), 'wrong')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/invalid/i)
    })
  })
})
