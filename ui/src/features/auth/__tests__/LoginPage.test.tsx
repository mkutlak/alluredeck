import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { LoginPage } from '../LoginPage'
import * as authApi from '@/api/auth'
import * as systemApi from '@/api/system'
import { mockApiClient } from '@/test/mocks/api-client'
import { useAuthStore } from '@/store/auth'

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

function renderLogin(route = '/login') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[route]}>
        <LoginPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function mockConfigResponse(oidcEnabled: boolean) {
  vi.mocked(systemApi.getConfig).mockResolvedValue({
    data: {
      version: '2.0',
      dev_mode: false,
      check_results_every_seconds: '30',
      keep_history: true,
      keep_history_latest: 25,
      tls: false,
      security_enabled: true,
      url_prefix: '',
      api_response_less_verbose: false,
      optimize_storage: false,
      make_viewer_endpoints_public: false,
      oidc_enabled: oidcEnabled,
    },
    metadata: { message: 'ok' },
  })
}

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().clearAuth()
  })

  describe('SSO button visibility', () => {
    it('renders SSO button when oidc_enabled is true', async () => {
      mockConfigResponse(true)

      renderLogin()

      await waitFor(() => {
        expect(screen.getByRole('link', { name: /sign in with sso/i })).toBeInTheDocument()
      })
    })

    it('does not render SSO button when oidc_enabled is false', async () => {
      mockConfigResponse(false)

      renderLogin()

      // Wait for config to load, then verify no SSO button
      await waitFor(() => {
        expect(systemApi.getConfig).toHaveBeenCalled()
      })
      expect(screen.queryByRole('link', { name: /sign in with sso/i })).not.toBeInTheDocument()
    })

    it('does not render SSO button while config is loading', () => {
      // Config never resolves
      vi.mocked(systemApi.getConfig).mockReturnValue(new Promise(() => {}))

      renderLogin()

      expect(screen.queryByRole('link', { name: /sign in with sso/i })).not.toBeInTheDocument()
    })
  })

  describe('SSO button navigation', () => {
    it('SSO button links to backend OIDC login URL', async () => {
      mockConfigResponse(true)

      renderLogin()

      const ssoButton = await screen.findByRole('link', { name: /sign in with sso/i })
      expect(ssoButton).toHaveAttribute('href', 'http://localhost:5050/auth/oidc/login')
    })
  })

  describe('OIDC callback handling', () => {
    it('calls getSession and populates auth store on ?oidc=success', async () => {
      mockConfigResponse(false)
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
        expect(authApi.getSession).toHaveBeenCalled()
      })

      await waitFor(() => {
        const state = useAuthStore.getState()
        expect(state.isAuthenticated).toBe(true)
        expect(state.username).toBe('sso-user')
        expect(state.provider).toBe('oidc')
      })

      expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true })
    })

    it('does not call getSession without ?oidc=success', async () => {
      mockConfigResponse(false)

      renderLogin('/login')

      await waitFor(() => {
        expect(systemApi.getConfig).toHaveBeenCalled()
      })
      expect(authApi.getSession).not.toHaveBeenCalled()
    })
  })

  describe('local login still works', () => {
    it('renders username and password fields', async () => {
      mockConfigResponse(false)

      renderLogin()

      expect(screen.getByLabelText(/username/i)).toBeInTheDocument()
      expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /sign in$/i })).toBeInTheDocument()
    })
  })
})
