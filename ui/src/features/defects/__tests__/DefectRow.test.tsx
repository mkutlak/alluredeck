import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { MemoryRouter } from 'react-router'
import { DefectRow } from '../DefectRow'
import type { DefectListRow } from '@/types/api'

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

function renderRow(props: Partial<Parameters<typeof DefectRow>[0]> = {}) {
  const defaultProps = {
    defect: makeDefect(),
    selected: false,
    onSelect: vi.fn(),
    onToggle: vi.fn(),
    expanded: false,
    projectId: 'myproject',
    ...props,
  }
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter>
        <DefectRow {...defaultProps} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('DefectRow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(defectsApi.fetchDefectTests).mockResolvedValue([])
  })

  it.each([
    { is_regression: true, is_new: false },
    { is_regression: false, is_new: true },
    { is_regression: false, is_new: false },
  ])('flags regression=$is_regression and new=$is_new defects', ({ is_regression, is_new }) => {
    renderRow({ defect: makeDefect({ is_regression, is_new }) })
    expect(screen.queryByTestId('regression-flag') !== null).toBe(is_regression)
    expect(screen.queryByTestId('new-flag') !== null).toBe(is_new)
  })

  it('toggles expansion on click', async () => {
    const onToggle = vi.fn()
    renderRow({ onToggle })
    await userEvent.click(screen.getByRole('button', { name: /toggle details/i }))
    expect(onToggle).toHaveBeenCalledWith('def-1')
  })

  it.each([true, false])('shows the detail panel only when expanded (%s)', (expanded) => {
    renderRow({ expanded })
    expect(screen.queryByTestId('defect-detail') !== null).toBe(expanded)
  })

  // Builds are numbered by build_order, never by builds.id (ids diverge on backfill).
  it('displays the test count and the build range by build_order, not build id', () => {
    renderRow({
      defect: makeDefect({
        test_result_count_in_build: 3,
        first_seen_build_id: 101,
        last_seen_build_id: 105,
        first_seen_build_order: 1,
        last_seen_build_order: 5,
      }),
    })
    expect(screen.getByText('3 tests')).toBeInTheDocument()
    expect(screen.getByText('#1–#5')).toBeInTheDocument()
  })
})
