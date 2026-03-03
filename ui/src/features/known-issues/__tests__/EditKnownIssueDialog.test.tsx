import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EditKnownIssueDialog } from '../EditKnownIssueDialog'
import * as kiApi from '@/api/known-issues'
import type { KnownIssue } from '@/types/api'

vi.mock('@/api/known-issues')
vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function makeIssue(overrides: Partial<KnownIssue> = {}): KnownIssue {
  return {
    id: 1,
    project_id: 'myproject',
    test_name: 'Login should succeed',
    pattern: '',
    ticket_url: '',
    description: '',
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderDialog(issue: KnownIssue, onOpenChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <EditKnownIssueDialog
        projectId="myproject"
        issue={issue}
        open={true}
        onOpenChange={onOpenChange}
      />
    </QueryClientProvider>,
  )
}

describe('EditKnownIssueDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('sends ticket_url as empty string (not undefined) when field is empty', async () => {
    const user = userEvent.setup()
    vi.mocked(kiApi.updateKnownIssue).mockResolvedValue(makeIssue())

    const issue = makeIssue({ ticket_url: '', description: 'Some desc', is_active: true })
    renderDialog(issue)

    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
        ticket_url: '',
        description: 'Some desc',
        is_active: true,
      })
    })
  })

  it('sends description as empty string (not undefined) when field is empty', async () => {
    const user = userEvent.setup()
    vi.mocked(kiApi.updateKnownIssue).mockResolvedValue(makeIssue())

    const issue = makeIssue({ ticket_url: 'https://jira.com/PROJ-1', description: '', is_active: true })
    renderDialog(issue)

    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
        ticket_url: 'https://jira.com/PROJ-1',
        description: '',
        is_active: true,
      })
    })
  })

  it('preserves all fields when only toggling is_active to false', async () => {
    const user = userEvent.setup()
    vi.mocked(kiApi.updateKnownIssue).mockResolvedValue(makeIssue({ is_active: false }))

    const issue = makeIssue({
      ticket_url: 'https://jira.com/PROJ-42',
      description: 'Flaky in CI',
      is_active: true,
    })
    renderDialog(issue)

    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
        ticket_url: 'https://jira.com/PROJ-42',
        description: 'Flaky in CI',
        is_active: false,
      })
    })
  })

  it('sends all fields with actual values when submitting with no changes', async () => {
    const user = userEvent.setup()
    vi.mocked(kiApi.updateKnownIssue).mockResolvedValue(makeIssue())

    const issue = makeIssue({
      ticket_url: 'https://jira.com/PROJ-1',
      description: 'Known flake',
      is_active: true,
    })
    renderDialog(issue)

    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
        ticket_url: 'https://jira.com/PROJ-1',
        description: 'Known flake',
        is_active: true,
      })
    })
  })
})
