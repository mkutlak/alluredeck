import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
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

  it.each([
    { state: 'loading', tests: () => new Promise<never>(() => {}), text: /loading tests/i },
    { state: 'empty', tests: () => Promise.resolve([]), text: /no test occurrences found/i },
    {
      state: 'error',
      tests: () => Promise.reject(new Error('boom')),
      text: /failed to load tests/i,
    },
  ])('shows the $state message for affected tests', async ({ tests, text }) => {
    vi.mocked(defectsApi.fetchDefectTests).mockImplementation(tests)
    renderDetail()
    expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it('renders test rows and a flaky badge for flaky occurrences only', async () => {
    vi.mocked(defectsApi.fetchDefectTests).mockResolvedValue([
      makeTestRow({ test_name: 'flaky test', flaky: true, retries: 2 }),
      makeTestRow({ test_name: 'stable test', flaky: false }),
    ])
    renderDetail()

    expect(await screen.findByText('flaky test')).toBeInTheDocument()
    expect(screen.getByText('stable test')).toBeInTheDocument()
    expect(screen.getByTestId('flaky-badge')).toHaveTextContent('flaky · 2x')
  })

  // Builds are numbered by build_order, never by builds.id (ids diverge on backfill).
  it('labels first and last seen builds by build_order, not build id', () => {
    vi.mocked(defectsApi.fetchDefectTests).mockResolvedValue([])
    renderDetail({
      first_seen_build_id: 101,
      last_seen_build_id: 105,
      first_seen_build_order: 1,
      last_seen_build_order: 5,
    })
    expect(screen.getByText('Build #1')).toBeInTheDocument()
    expect(screen.getByText('Build #5')).toBeInTheDocument()
  })
})
