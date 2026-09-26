import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { MemoryRouter } from 'react-router'
import { DefectList } from '../DefectList'
import * as defectsApi from '@/api/defects'
import type { DefectListResponse, DefectListRow } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/defects')
mockApiClient()

function makeResponse(data: DefectListRow[] = []): DefectListResponse {
  return {
    data,
    metadata: { message: 'ok' },
    pagination: { total: data.length, page: 1, per_page: 25, total_pages: data.length ? 1 : 0 },
  }
}

const defect: DefectListRow = {
  id: 'def-1',
  project_id: 1,
  fingerprint_hash: 'abc123',
  normalized_message: 'NullPointerException in UserService',
  sample_trace: '',
  category: 'product_bug',
  resolution: 'open',
  known_issue_id: null,
  first_seen_build_id: 1,
  last_seen_build_id: 3,
  occurrence_count: 5,
  consecutive_clean_builds: 0,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  test_result_count_in_build: 2,
  first_seen_build_order: 1,
  last_seen_build_order: 3,
  is_regression: false,
  is_new: true,
  known_issue: null,
}

function renderList(props: Partial<Parameters<typeof DefectList>[0]> = {}) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter>
        <DefectList projectId="myproject" {...props} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('DefectList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows the error state and retries into the empty state', async () => {
    const user = userEvent.setup()
    vi.mocked(defectsApi.fetchProjectDefects)
      .mockRejectedValueOnce(new Error('Network error'))
      .mockResolvedValue(makeResponse())
    renderList()

    expect(await screen.findByText(/couldn't load data/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /retry/i }))
    expect(await screen.findByText(/No defects found/i)).toBeInTheDocument()
  })

  it.each([
    {
      scope: 'project',
      props: {},
      fetcher: 'fetchProjectDefects' as const,
      other: 'fetchBuildDefects' as const,
    },
    {
      scope: 'build',
      props: { buildId: 42 },
      fetcher: 'fetchBuildDefects' as const,
      other: 'fetchProjectDefects' as const,
    },
  ])('lists $scope defects from $fetcher', async ({ props, fetcher, other }) => {
    vi.mocked(defectsApi[fetcher]).mockResolvedValue(makeResponse([defect]))
    renderList(props)

    expect(await screen.findByText('NullPointerException in UserService')).toBeInTheDocument()
    expect(defectsApi[other]).not.toHaveBeenCalled()
  })
})
