import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router'
import { KnownIssuesTab } from '../KnownIssuesTab'
import * as kiApi from '@/api/known-issues'
import { useAuthStore } from '@/store/auth'

vi.mock('@/api/known-issues')
vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function renderTab(projectId = 'myproject', isAdminUser = true) {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: isAdminUser ? ['admin'] : ['viewer'],
    username: isAdminUser ? 'admin' : 'viewer',
    expiresAt: Date.now() + 3_600_000,
  })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[`/projects/${projectId}/known-issues`]}>
        <Routes>
          <Route path="projects/:id/known-issues" element={<KnownIssuesTab />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('KnownIssuesTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeletons initially', () => {
    vi.mocked(kiApi.listKnownIssues).mockReturnValue(new Promise(() => {}))
    renderTab()
    expect(screen.getByText('Known Issues')).toBeInTheDocument()
  })

  it('shows empty state when no issues', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([])
    renderTab()
    await waitFor(() => {
      expect(screen.getByText(/No known issues tracked/i)).toBeInTheDocument()
    })
  })

  it('renders issues in table', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([
      {
        id: 1,
        project_id: 'myproject',
        test_name: 'Login should succeed',
        pattern: '',
        ticket_url: 'http://ticket/1',
        description: 'Flaky',
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ])
    renderTab()
    await waitFor(() => {
      expect(screen.getByText('Login should succeed')).toBeInTheDocument()
      expect(screen.getByText('active')).toBeInTheDocument()
    })
  })

  it('shows Add Known Issue button for admin', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([])
    renderTab()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Add Known Issue/i })).toBeInTheDocument()
    })
  })

  it('hides Add Known Issue button for viewer', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([])
    renderTab('myproject', false)
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /Add Known Issue/i })).not.toBeInTheDocument()
    })
  })
})
