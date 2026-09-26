import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, type InitialEntry } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { LoginPage } from '../LoginPage'
import * as authApi from '@/api/auth'
import * as systemApi from '@/api/system'
import { mockApiClient } from '@/test/mocks/api-client'
import { useAuthStore } from '@/store/auth'
import type { ApiResponse, ConfigData } from '@/types/api'

vi.mock('@/api/auth')
vi.mock('@/api/system')
mockApiClient()

const mockNavigate = vi.fn()
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

function config(oidcEnabled: boolean): ApiResponse<ConfigData> {
  return { data: { oidc_enabled: oidcEnabled } as ConfigData, metadata: { message: 'ok' } }
}

// `seeded` pre-fills the /config cache (fresh for its 5-minute staleTime), so the
// first render already reflects it.
function renderLogin(entry: InitialEntry = '/login', seeded?: ApiResponse<ConfigData>) {
  const qc = createTestQueryClient()
  if (seeded) qc.setQueryData(['config'], seeded)
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[entry]}>
        <LoginPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

async function signIn(username: string, password: string) {
  const user = userEvent.setup()
  if (username) await user.type(screen.getByLabelText(/username/i), username)
  if (password) await user.type(screen.getByLabelText(/password/i), password)
  await user.click(screen.getByRole('button', { name: /sign in$/i }))
}

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().clearAuth()
    vi.mocked(systemApi.getConfig).mockReturnValue(new Promise(() => {}))
  })

  describe('local login', () => {
    it.each([
      { name: 'no redirect state', from: undefined, want: '/' },
      { name: 'a valid internal path', from: '/dashboard', want: '/dashboard' },
      // Open-redirect guard: a protocol-relative URL is not an internal path.
      { name: 'a protocol-relative URL (//evil.com)', from: '//evil.com', want: '/' },
    ])('logs in and navigates to $want for $name', async ({ from, want }) => {
      vi.mocked(authApi.login).mockResolvedValue({
        data: { csrf_token: 'csrf123', expires_in: 3600, roles: ['admin'] },
        metadata: { message: 'ok' },
      })
      renderLogin(from ? { pathname: '/login', state: { from: { pathname: from } } } : '/login')

      await signIn('admin', 'secret')

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith(want, { replace: true })
      })
      // TanStack Query v5 passes an internal context object as the second arg to mutationFn.
      expect(authApi.login).toHaveBeenCalledWith(
        { username: 'admin', password: 'secret' },
        expect.anything(),
      )
      expect(useAuthStore.getState().isAuthenticated).toBe(true)
    })

    it.each([
      { name: 'fields are empty', username: '', reject: false, alert: /required/i },
      { name: 'the API rejects', username: 'admin', reject: true, alert: /invalid/i },
    ])('shows an error alert when $name', async ({ username, reject, alert }) => {
      if (reject) vi.mocked(authApi.login).mockRejectedValue(new Error('Invalid username/password'))
      renderLogin()

      await signIn(username, username && 'wrong')

      expect(await screen.findByRole('alert')).toHaveTextContent(alert)
      expect(mockNavigate).not.toHaveBeenCalled()
    })
  })

  describe('SSO', () => {
    it('links the SSO button to the backend OIDC login URL when oidc_enabled is true', async () => {
      vi.mocked(systemApi.getConfig).mockResolvedValue(config(true))
      renderLogin()

      const ssoButton = await screen.findByRole('link', { name: /sign in with sso/i })
      expect(ssoButton).toHaveAttribute('href', 'http://localhost:5050/auth/oidc/login')
    })

    it.each([
      { name: 'oidc_enabled is false', seeded: config(false) },
      { name: 'config is still loading', seeded: undefined },
    ])('does not render the SSO button when $name', ({ seeded }) => {
      renderLogin('/login', seeded)
      expect(screen.queryByRole('link', { name: /sign in with sso/i })).not.toBeInTheDocument()
    })
  })

  describe('OIDC callback handling', () => {
    it('calls getSession and populates auth store on ?oidc=success', async () => {
      vi.mocked(authApi.getSession).mockResolvedValue({
        data: {
          username: 'sso-user',
          roles: ['editor'],
          expires_in: 3600,
          provider: 'oidc',
        },
        metadata: { message: 'ok' },
      })

      renderLogin('/login?oidc=success')

      await waitFor(() => {
        const state = useAuthStore.getState()
        expect(state.isAuthenticated).toBe(true)
        expect(state.username).toBe('sso-user')
        expect(state.provider).toBe('oidc')
      })
      expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true })
    })

    it('does not call getSession without ?oidc=success', () => {
      renderLogin('/login')
      expect(authApi.getSession).not.toHaveBeenCalled()
    })
  })
})
