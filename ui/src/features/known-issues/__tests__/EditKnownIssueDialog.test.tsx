import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { EditKnownIssueDialog } from '../EditKnownIssueDialog'
import * as kiApi from '@/api/known-issues'
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
    ticket_url: '',
    description: '',
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('EditKnownIssueDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // Empty fields are sent as '' (not undefined) and untouched fields keep their values.
  it.each([
    { name: 'empty ticket_url', ticket_url: '', description: 'Some desc', uncheck: false },
    {
      name: 'empty description',
      ticket_url: 'https://jira.com/PROJ-1',
      description: '',
      uncheck: false,
    },
    {
      name: 'only is_active toggled off',
      ticket_url: 'https://jira.com/PROJ-42',
      description: 'Flaky in CI',
      uncheck: true,
    },
  ])('saves every field with $name', async ({ ticket_url, description, uncheck }) => {
    const user = userEvent.setup()
    const issue = makeIssue({ ticket_url, description, is_active: true })
    vi.mocked(kiApi.updateKnownIssue).mockResolvedValue(issue)
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <EditKnownIssueDialog
          projectId="myproject"
          issue={issue}
          open={true}
          onOpenChange={vi.fn()}
        />
      </QueryClientProvider>,
    )

    if (uncheck) await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(kiApi.updateKnownIssue).toHaveBeenCalledWith('myproject', 1, {
        ticket_url,
        description,
        is_active: !uncheck,
      })
    })
  })
})
