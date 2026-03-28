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

function makeDefect(overrides: Partial<DefectListRow> = {}): DefectListRow {
  return {
    id: 'def-1',
    project_id: 'myproject',
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
  })

  it('renders normalized message', () => {
    renderRow()
    expect(screen.getByText('NullPointerException in UserService.getUser')).toBeInTheDocument()
  })

  it('renders category badge', () => {
    renderRow()
    expect(screen.getByTestId('category-badge')).toHaveTextContent('Product Bug')
  })

  it('renders different category badges', () => {
    renderRow({ defect: makeDefect({ category: 'test_bug' }) })
    expect(screen.getByTestId('category-badge')).toHaveTextContent('Test Bug')
  })

  it('shows regression flag when is_regression is true', () => {
    renderRow({ defect: makeDefect({ is_regression: true }) })
    expect(screen.getByTestId('regression-flag')).toBeInTheDocument()
  })

  it('shows new flag when is_new is true', () => {
    renderRow({ defect: makeDefect({ is_new: true }) })
    expect(screen.getByTestId('new-flag')).toBeInTheDocument()
  })

  it('does not show flags when both are false', () => {
    renderRow()
    expect(screen.queryByTestId('regression-flag')).not.toBeInTheDocument()
    expect(screen.queryByTestId('new-flag')).not.toBeInTheDocument()
  })

  it('toggles expansion on click', async () => {
    const onToggle = vi.fn()
    renderRow({ onToggle })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /toggle details/i }))
    expect(onToggle).toHaveBeenCalledWith('def-1')
  })

  it('shows detail panel when expanded', () => {
    renderRow({ expanded: true })
    expect(screen.getByTestId('defect-detail')).toBeInTheDocument()
  })

  it('hides detail panel when collapsed', () => {
    renderRow({ expanded: false })
    expect(screen.queryByTestId('defect-detail')).not.toBeInTheDocument()
  })

  it('displays test count', () => {
    renderRow({ defect: makeDefect({ test_result_count_in_build: 3 }) })
    expect(screen.getByText('3 tests')).toBeInTheDocument()
  })

  it('displays build range', () => {
    renderRow()
    expect(screen.getByText('#1–#5')).toBeInTheDocument()
  })
})
