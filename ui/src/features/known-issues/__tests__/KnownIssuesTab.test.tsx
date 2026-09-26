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

function renderTab(isAdminUser = true) {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: isAdminUser ? ['admin'] : ['viewer'],
    username: isAdminUser ? 'admin' : 'viewer',
    expiresAt: Date.now() + 3_600_000,
  })
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/projects/myproject/known-issues']}>
        <Routes>
          <Route path="projects/:id/known-issues" element={<KnownIssuesTab />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const toggleButton = { name: /toggle issue status/i }

describe('KnownIssuesTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows the error state and retries into the empty state', async () => {
    const user = userEvent.setup()
    vi.mocked(kiApi.listKnownIssues)
      .mockRejectedValueOnce(new Error('Network error'))
      .mockResolvedValue([])
    renderTab()

    expect(await screen.findByText(/couldn't load data/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /retry/i }))
    expect(await screen.findByText(/No known issues tracked/i)).toBeInTheDocument()
  })

  it.each([
    { role: 'admin', admin: true },
    { role: 'viewer', admin: false },
  ])('shows add and status-toggle buttons only to editors ($role)', async ({ admin }) => {
    vi.mocked(kiApi.listKnownIssues).mockResolvedValue([makeIssue()])
    renderTab(admin)

    expect(await screen.findByText('Login should succeed')).toBeInTheDocument()
    expect(screen.getByText('active')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Add Known Issue/i }) !== null).toBe(admin)
    expect(screen.queryByRole('button', toggleButton) !== null).toBe(admin)
  })

  describe('XSS protection', () => {
    it.each([
      { url: 'javascript:alert(1)', link: false },
      { url: 'https://jira.com/PROJ-1', link: true },
    ])('renders ticket_url $url as a link: $link', async ({ url, link }) => {
      vi.mocked(kiApi.listKnownIssues).mockResolvedValue([makeIssue({ ticket_url: url })])
      renderTab()

      await screen.findByText('Login should succeed')
      // No anchor at all for an unsafe URL: React would only neuter the href, not drop the link.
      const hrefs = screen.queryAllByRole('link').map((l) => l.getAttribute('href'))
      expect(hrefs).toEqual(link ? [url] : [])
    })
  })

  describe('inline toggle', () => {
    it.each([
      {
        name: 'resolves an active issue',
        issue: makeIssue({ ticket_url: 'https://jira.com/PROJ-1', description: 'Flaky in CI' }),
        showResolved: false,
      },
      {
        name: 'reactivates a resolved issue',
        issue: makeIssue({ ticket_url: '', description: '', is_active: false }),
        showResolved: true,
      },
    ])('$name, preserving the other fields', async ({ issue, showResolved }) => {
      const user = userEvent.setup()
      vi.mocked(kiApi.listKnownIssues).mockResolvedValue([issue])
      vi.mocked(kiApi.updateKnownIssue).mockResolvedValue({ ...issue, is_active: !issue.is_active })
      renderTab()

      if (showResolved) {
        await user.click(screen.getByLabelText(/show resolved/i))
        await waitFor(() => {
          expect(kiApi.listKnownIssues).toHaveBeenLastCalledWith('myproject', false)
        })
      }
      await user.click(await screen.findByRole('button', toggleButton))

      await waitFor(() => {
        expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
          ticket_url: issue.ticket_url,
          description: issue.description,
          is_active: !issue.is_active,
        })
      })
    })
  })
})
