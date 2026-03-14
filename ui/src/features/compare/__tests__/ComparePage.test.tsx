import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { ComparePage } from '../ComparePage'
import * as reportsApi from '@/api/reports'
import type { CompareData } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function makeCompareData(overrides: Partial<CompareData> = {}): CompareData {
  return {
    build_a: 1,
    build_b: 2,
    summary: { regressed: 1, fixed: 1, added: 1, removed: 0, total: 3 },
    tests: [
      {
        test_name: 'LoginTest',
        full_name: 'pkg.LoginTest',
        history_id: 'h1',
        status_a: 'passed',
        status_b: 'failed',
        duration_a: 1000,
        duration_b: 2000,
        duration_delta: 1000,
        category: 'regressed',
      },
      {
        test_name: 'SignupTest',
        full_name: 'pkg.SignupTest',
        history_id: 'h2',
        status_a: 'failed',
        status_b: 'passed',
        duration_a: 500,
        duration_b: 300,
        duration_delta: -200,
        category: 'fixed',
      },
      {
        test_name: 'NewTest',
        full_name: 'pkg.NewTest',
        history_id: 'h3',
        status_a: '',
        status_b: 'passed',
        duration_a: 0,
        duration_b: 400,
        duration_delta: 400,
        category: 'added',
      },
    ],
    ...overrides,
  }
}

function renderPage(search = '?a=1&b=2') {
  const router = createMemoryRouter([{ path: '/projects/:id/compare', element: <ComparePage /> }], {
    initialEntries: [`/projects/test-project/compare${search}`],
  })
  return renderWithProviders(<></>, { router })
}

describe('ComparePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders summary cards with correct counts', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(makeCompareData())
    renderPage()

    // Filter buttons display category labels with counts — unique text, avoids multi-match
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /regressed.*1/i })).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: /fixed.*1/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /added.*1/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /removed.*0/i })).toBeInTheDocument()
  })

  it('renders diff table rows', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(makeCompareData())
    renderPage()

    await waitFor(() => {
      // Test names appear in table cells — use getAllByText since span + td share textContent
      expect(screen.getAllByText('LoginTest').length).toBeGreaterThan(0)
    })
    expect(screen.getAllByText('SignupTest').length).toBeGreaterThan(0)
    expect(screen.getAllByText('NewTest').length).toBeGreaterThan(0)
  })

  it('category filter shows only matching rows', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(makeCompareData())
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText('LoginTest').length).toBeGreaterThan(0)
    })

    const user = userEvent.setup()
    // Click the "Fixed" filter button (text: "Fixed (1)")
    await user.click(screen.getByRole('button', { name: /^fixed/i }))

    await waitFor(() => {
      expect(screen.getAllByText('SignupTest').length).toBeGreaterThan(0)
    })
    expect(screen.queryByText('LoginTest')).not.toBeInTheDocument()
    expect(screen.queryByText('NewTest')).not.toBeInTheDocument()
  })

  it('all filter restores all rows', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(makeCompareData())
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText('LoginTest').length).toBeGreaterThan(0)
    })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /^fixed/i }))
    await waitFor(() => {
      expect(screen.queryByText('LoginTest')).not.toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /^all/i }))
    await waitFor(() => {
      expect(screen.getAllByText('LoginTest').length).toBeGreaterThan(0)
    })
    expect(screen.getAllByText('SignupTest').length).toBeGreaterThan(0)
  })

  it('shows loading state while fetching', () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockReturnValue(new Promise(() => {}))
    renderPage()

    const skeletons = document.querySelectorAll('[data-testid="compare-skeleton"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('shows error message when params are missing', () => {
    renderPage('') // no query params
    // Error message is in a <p> element — use selector to avoid multi-match with ancestors
    expect(screen.getByText(/invalid/i, { selector: 'p' })).toBeInTheDocument()
  })

  it('shows error message when params are invalid', () => {
    renderPage('?a=foo&b=bar')
    expect(screen.getByText(/invalid/i, { selector: 'p' })).toBeInTheDocument()
  })

  it('shows error message when param is partial-numeric (e.g. 42abc)', () => {
    renderPage('?a=42abc&b=2')
    expect(screen.getByText(/invalid/i, { selector: 'p' })).toBeInTheDocument()
  })

  it('shows title with build numbers', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(makeCompareData())
    renderPage()

    await waitFor(() => {
      // Use heading role to target the <h1> specifically
      expect(screen.getByRole('heading', { name: /build #1/i })).toBeInTheDocument()
      expect(screen.getByRole('heading', { name: /build #2/i })).toBeInTheDocument()
    })
  })

  it('has a back navigation link', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(makeCompareData())
    renderPage()

    await waitFor(() => {
      const backLink = screen.getByRole('link', { name: /back/i })
      expect(backLink).toBeInTheDocument()
    })
  })

  it('shows empty state when no diffs', async () => {
    vi.mocked(reportsApi.fetchBuildComparison).mockResolvedValue(
      makeCompareData({
        summary: { regressed: 0, fixed: 0, added: 0, removed: 0, total: 0 },
        tests: [],
      }),
    )
    renderPage()

    await waitFor(() => {
      // Use selector:'p' to avoid matching ancestor elements with same textContent
      expect(screen.getByText(/no differences/i, { selector: 'p' })).toBeInTheDocument()
    })
  })
})
