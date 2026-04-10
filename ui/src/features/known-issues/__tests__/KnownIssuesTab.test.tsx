import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { MemoryRouter, Route, Routes } from 'react-router'
import { KnownIssuesTab } from '../KnownIssuesTab'
import * as kiApi from '@/api/known-issues'
import { useAuthStore } from '@/store/auth'
import type { KnownIssue } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/known-issues')
mockApiClient()

function makeIssue(overrides: Partial<KnownIssue> = {}): KnownIssue {
  return {
    id: 1,
    project_id: 1,
    test_name: 'Login should succeed',
    pattern: '',
    ticket_url: 'https://jira.com/PROJ-1',
    description: 'Flaky in CI',
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderTab(projectId = 'myproject', isAdminUser = true) {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: isAdminUser ? ['admin'] : ['viewer'],
    username: isAdminUser ? 'admin' : 'viewer',
    expiresAt: Date.now() + 3_600_000,
  })
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
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
        project_id: 1,
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

describe('KnownIssuesTab – XSS protection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not render javascript: ticket_url as a link', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([
      makeIssue({ ticket_url: 'javascript:alert(1)' }),
    ])
    renderTab()
    await waitFor(() => {
      expect(screen.getByText('Login should succeed')).toBeInTheDocument()
    })
    // The ticket_url should NOT be rendered as an <a> link
    const links = screen.queryAllByRole('link')
    const dangerousLink = links.find((l) => l.getAttribute('href') === 'javascript:alert(1)')
    expect(dangerousLink).toBeUndefined()
  })

  it('renders safe https ticket_url as a link', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([
      makeIssue({ ticket_url: 'https://jira.com/PROJ-1' }),
    ])
    renderTab()
    await waitFor(() => {
      expect(screen.getByText('Login should succeed')).toBeInTheDocument()
    })
    const link = screen.getByRole('link', { name: /jira\.com/i })
    expect(link).toHaveAttribute('href', 'https://jira.com/PROJ-1')
  })
})

describe('KnownIssuesTab – inline toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders toggle status button for admin users', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([makeIssue()])
    renderTab()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /toggle issue status/i })).toBeInTheDocument()
    })
  })

  it('hides toggle status button for viewer users', async () => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([makeIssue()])
    renderTab('myproject', false)
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /toggle issue status/i })).not.toBeInTheDocument()
    })
  })

  it('clicking toggle calls updateKnownIssue with flipped is_active and preserved fields', async () => {
    const user = userEvent.setup()
    const issue = makeIssue({
      ticket_url: 'https://jira.com/PROJ-1',
      description: 'Flaky in CI',
      is_active: true,
    })
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([issue])
    vi.mocked(kiApi.updateKnownIssue).mockResolvedValue({ ...issue, is_active: false })

    renderTab()
    await waitFor(() => screen.getByRole('button', { name: /toggle issue status/i }))

    await user.click(screen.getByRole('button', { name: /toggle issue status/i }))

    await waitFor(() => {
      expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
        ticket_url: 'https://jira.com/PROJ-1',
        description: 'Flaky in CI',
        is_active: false,
      })
    })
  })

  it('toggle sends is_active: true when issue is currently resolved', async () => {
    const user = userEvent.setup()
    const issue = makeIssue({ ticket_url: '', description: '', is_active: false })
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([issue])
    vi.mocked(kiApi.updateKnownIssue).mockResolvedValue({ ...issue, is_active: true })

    renderTab('myproject', true)
    // Show resolved issues so the inactive one is visible
    await waitFor(() => screen.getByLabelText(/show resolved/i))
    await user.click(screen.getByLabelText(/show resolved/i))
    await waitFor(() => screen.getByRole('button', { name: /toggle issue status/i }))

    await user.click(screen.getByRole('button', { name: /toggle issue status/i }))

    await waitFor(() => {
      expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
        ticket_url: '',
        description: '',
        is_active: true,
      })
    })
  })
})
