import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { ComparePage } from '../ComparePage'
import * as reportsApi from '@/api/reports'
import type { CompareData, DiffCategory } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function diff(test_name: string, category: DiffCategory) {
  return {
    test_name,
    full_name: `pkg.${test_name}`,
    history_id: test_name,
    status_a: 'passed',
    status_b: 'failed',
    duration_a: 1000,
    duration_b: 2000,
    duration_delta: 1000,
    category,
  }
}

function makeCompareData(overrides: Partial<CompareData> = {}): CompareData {
  return {
    build_a: 1,
    build_b: 2,
    summary: { regressed: 1, fixed: 1, added: 1, removed: 0, total: 3 },
    tests: [diff('LoginTest', 'regressed'), diff('SignupTest', 'fixed'), diff('NewTest', 'added')],
    ...overrides,
  }
}

function renderPage(search = '?a=1&b=2') {
  const router = createMemoryRouter([{ path: '/projects/:id/compare', element: <ComparePage /> }], {
    initialEntries: [`/projects/test-project/compare${search}`],
  })
  return renderWithProviders(<></>, { router })
}

// Test names appear in a span and its cell, so match on any occurrence.
const shows = (name: string) => screen.queryAllByText(name).length > 0

describe('ComparePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('counts each category and filters the diff rows by it', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(makeCompareData())
    renderPage()

    await screen.findByRole('button', { name: /regressed.*1/i })
    expect(screen.getByRole('button', { name: /fixed.*1/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /added.*1/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /removed.*0/i })).toBeInTheDocument()
    expect(['LoginTest', 'SignupTest', 'NewTest'].map(shows)).toEqual([true, true, true])

    await user.click(screen.getByRole('button', { name: /^fixed/i }))
    expect(['LoginTest', 'SignupTest', 'NewTest'].map(shows)).toEqual([false, true, false])

    await user.click(screen.getByRole('button', { name: /^all/i }))
    expect(['LoginTest', 'SignupTest', 'NewTest'].map(shows)).toEqual([true, true, true])
  })

  it.each([
    ['shows error message when params are missing', ''],
    ['shows error message when params are invalid', '?a=foo&b=bar'],
    ['shows error message when param is partial-numeric (e.g. 42abc)', '?a=42abc&b=2'],
  ])('%s', (_name, search) => {
    renderPage(search)
    expect(screen.getByText(/invalid/i, { selector: 'p' })).toBeInTheDocument()
  })

  it('shows empty state when no diffs', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(
      makeCompareData({
        summary: { regressed: 0, fixed: 0, added: 0, removed: 0, total: 0 },
        tests: [],
      }),
    )
    renderPage()
    expect(await screen.findByText(/no differences/i, { selector: 'p' })).toBeInTheDocument()
  })
})
