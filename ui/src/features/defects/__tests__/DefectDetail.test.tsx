import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { DefectDetail } from '../DefectDetail'
import type { DefectListRow, DefectTestRow } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/defects')
mockApiClient()

import * as defectsApi from '@/api/defects'

function makeDefect(overrides: Partial<DefectListRow> = {}): DefectListRow {
  return {
    id: 'def-1',
    project_id: 1,
    fingerprint_hash: 'abc123',
    normalized_message: 'NullPointerException in UserService.getUser',
    sample_trace: 'java.lang.NullPointerException\n  at UserService.getUser(UserService.java:42)',
    category: 'product_bug',
    resolution: 'open',
    known_issue_id: null,
    first_seen_build_id: 1,
    last_seen_build_id: 5,
    occurrence_count: 12,
    consecutive_clean_builds: 0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    test_result_count_in_build: 3,
    first_seen_build_order: 1,
    last_seen_build_order: 5,
    is_regression: false,
    is_new: false,
    known_issue: null,
    ...overrides,
  }
}

function makeTestRow(overrides: Partial<DefectTestRow> = {}): DefectTestRow {
  return {
    build_id: 5,
    test_name: 'should login',
    full_name: 'suite.should login',
    status: 'failed',
    history_id: 'h1',
    duration_ms: 120,
    flaky: false,
    retries: 0,
    new_failed: false,
    new_passed: false,
    status_message: 'Expected true to be false',
    ...overrides,
  }
}

function renderDetail(overrides: Partial<DefectListRow> = {}) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <DefectDetail defect={makeDefect(overrides)} projectId="myproject" />
    </QueryClientProvider>,
  )
}

describe('DefectDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows a loading message while tests are fetching', () => {
    vi.mocked(defectsApi.fetchDefectTests).mockReturnValue(new Promise(() => {}))
    renderDetail()
    expect(screen.getByText(/loading tests/i)).toBeInTheDocument()
  })

  it('shows an empty message when no test occurrences exist', async () => {
    vi.mocked(defectsApi.fetchDefectTests).mockResolvedValue([])
    renderDetail()
    await waitFor(() => {
      expect(screen.getByText(/no test occurrences found/i)).toBeInTheDocument()
    })
  })

  it('shows an error message when the tests fetch fails', async () => {
    vi.mocked(defectsApi.fetchDefectTests).mockRejectedValue(new Error('boom'))
    renderDetail()
    await waitFor(() => {
      expect(screen.getByText(/failed to load tests/i)).toBeInTheDocument()
    })
  })

  it('renders test rows and a flaky badge for flaky occurrences', async () => {
    vi.mocked(defectsApi.fetchDefectTests).mockResolvedValue([
      makeTestRow({ test_name: 'flaky test', flaky: true, retries: 2 }),
      makeTestRow({ test_name: 'stable test', flaky: false }),
    ])
    renderDetail()

    await waitFor(() => {
      expect(screen.getByText('flaky test')).toBeInTheDocument()
    })
    expect(screen.getByText('stable test')).toBeInTheDocument()
    expect(screen.getByTestId('flaky-badge')).toHaveTextContent('flaky · 2x')
  })

  it('does not render a flaky badge for non-flaky rows', async () => {
    vi.mocked(defectsApi.fetchDefectTests).mockResolvedValue([makeTestRow({ flaky: false })])
    renderDetail()

    await waitFor(() => {
      expect(screen.getByText('should login')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('flaky-badge')).not.toBeInTheDocument()
  })

  it('renders defect metadata', () => {
    vi.mocked(defectsApi.fetchDefectTests).mockResolvedValue([])
    renderDetail()
    expect(screen.getByText('Build #1')).toBeInTheDocument()
    expect(screen.getByText('Build #5')).toBeInTheDocument()
  })
})
