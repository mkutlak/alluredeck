import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { MemoryRouter } from 'react-router'
import { DefectList } from '../DefectList'
import * as defectsApi from '@/api/defects'
import type { DefectListResponse } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/defects')
mockApiClient()

function makeEmptyResponse(): DefectListResponse {
  return {
    data: [],
    metadata: { message: 'ok' },
    pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
  }
}

function renderList(props: Partial<Parameters<typeof DefectList>[0]> = {}) {
  const defaultProps = {
    projectId: 'myproject',
    ...props,
  }
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter>
        <DefectList {...defaultProps} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('DefectList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders empty state when no defects', async () => {
    vi.mocked(defectsApi.fetchProjectDefects).mockResolvedValue(makeEmptyResponse())
    renderList()
    await waitFor(() => {
      expect(screen.getByText(/No defects found/i)).toBeInTheDocument()
    })
  })

  it('shows loading skeletons initially', () => {
    vi.mocked(defectsApi.fetchProjectDefects).mockReturnValue(new Promise(() => {}))
    renderList()
    // Skeletons are rendered as placeholders
    const container = document.querySelector('.space-y-2')
    expect(container).toBeInTheDocument()
  })

  it('shows error state on fetch failure', async () => {
    vi.mocked(defectsApi.fetchProjectDefects).mockRejectedValue(new Error('Network error'))
    renderList()
    await waitFor(() => {
      expect(screen.getByText(/Failed to load defects/i)).toBeInTheDocument()
    })
  })

  it('renders defect rows when data is returned', async () => {
    vi.mocked(defectsApi.fetchProjectDefects).mockResolvedValue({
      data: [
        {
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
        },
      ],
      metadata: { message: 'ok' },
      pagination: { total: 1, page: 1, per_page: 25, total_pages: 1 },
    })
    renderList()
    await waitFor(() => {
      expect(screen.getByText('NullPointerException in UserService')).toBeInTheDocument()
    })
  })

  it('uses fetchBuildDefects when buildId is provided', async () => {
    vi.mocked(defectsApi.fetchBuildDefects).mockResolvedValue(makeEmptyResponse())
    renderList({ buildId: 42 })
    await waitFor(() => {
      expect(defectsApi.fetchBuildDefects).toHaveBeenCalled()
    })
    expect(defectsApi.fetchProjectDefects).not.toHaveBeenCalled()
  })
})
