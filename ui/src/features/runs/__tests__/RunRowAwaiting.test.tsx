import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import type { PipelineRun, PipelineSuite } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 10, slug: 'acme', display_name: 'Acme', parent_id: null, children: [1, 2] },
    ],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

function makeSuite(overrides?: Partial<PipelineSuite>): PipelineSuite {
  return {
    project_id: 1,
    slug: 'api-cloud',
    build_number: 5,
    build_id: 105,
    pass_rate: 100,
    total: 42,
    failed: 0,
    duration_ms: 15000,
    status: 'passed',
    ...overrides,
  }
}

// Two suites arrived; `awaiting` more are expected.
function passingRun(awaiting?: number): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [makeSuite(), makeSuite({ project_id: 2, slug: 'ui-tests' })],
    aggregate: {
      suites_passed: 2,
      suites_total: 2,
      tests_passed: 84,
      tests_total: 84,
      pass_rate: 100,
      total_duration_ms: 30000,
      awaiting_suites: awaiting,
    },
  }
}

function failingRun(awaiting?: number): PipelineRun {
  return {
    ...passingRun(awaiting),
    suites: [
      makeSuite(),
      makeSuite({ project_id: 2, slug: 'ui-tests', failed: 15, total: 100, status: 'degraded' }),
    ],
    aggregate: {
      suites_passed: 1,
      suites_total: 2,
      tests_passed: 127,
      tests_total: 142,
      pass_rate: 89.4,
      total_duration_ms: 45000,
      awaiting_suites: awaiting,
    },
  }
}

const summary = () => screen.getByTestId('run-row-summary')
const progressGlyph = () => screen.getByRole('img', { name: /in progress/i })

describe('RunRow for a pipeline run that is still uploading', () => {
  it.each([
    { awaiting: 3, text: 'awaiting 3 suites' },
    { awaiting: 1, text: 'awaiting 1 suite' },
  ])('reads $text instead of a suites-passed verdict', ({ awaiting, text }) => {
    renderWithProviders(<RunRow run={passingRun(awaiting)} />)
    expect(summary().textContent).toBe(text)
  })

  it('has no "suites passed" or "suites failing" wording, passing or failing', () => {
    const { unmount } = renderWithProviders(<RunRow run={passingRun(2)} />)
    expect(summary()).not.toHaveTextContent(/suites? (passed|failing)/)
    unmount()

    renderWithProviders(<RunRow run={failingRun(2)} />)
    expect(summary()).not.toHaveTextContent(/suites? (passed|failing)/)
  })

  it('shows the neutral ○ named "in progress": no green check, no red cross, no verdict colour', () => {
    renderWithProviders(<RunRow run={passingRun(3)} />)
    const row = screen.getByTestId('run-row')
    expect(progressGlyph()).toHaveTextContent('○')
    expect(row).not.toHaveTextContent('✓')
    expect(row).not.toHaveTextContent('✗')
    expect(row.outerHTML).not.toMatch(/d20f39|f38ba8|40a02b|a6e3a1/)
  })

  it('keeps every fact that has arrived when something failed', () => {
    renderWithProviders(<RunRow run={failingRun(2)} />)

    expect(summary().textContent).toBe('awaiting 2 suites · 15 failed tests')
    expect(screen.getByText('15 failed tests')).toHaveClass('text-[#d20f39]')
    expect(progressGlyph()).toHaveTextContent('○')
    expect(progressGlyph().className).not.toMatch(/d20f39|f38ba8|40a02b|a6e3a1/)
    // Second line, expander and pass rate are about what arrived, so they stay.
    expect(screen.getByRole('link', { name: /ui-tests/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /toggle failures/i })).toBeInTheDocument()
    expect(screen.getByText('89.4%')).toBeInTheDocument()
  })

  it.each([undefined, 0])('keeps the usual wording when awaiting_suites is %s', (awaiting) => {
    const { unmount } = renderWithProviders(<RunRow run={passingRun(awaiting)} />)
    expect(summary().textContent).toBe('2/2 suites passed')
    expect(screen.getByTestId('run-row')).toHaveTextContent('✓')
    expect(screen.queryByRole('img', { name: /in progress/i })).not.toBeInTheDocument()
    unmount()

    renderWithProviders(<RunRow run={failingRun(awaiting)} />)
    expect(summary().textContent).toBe('1/2 suites failing · 15 failed tests')
    expect(screen.getByTestId('run-row')).toHaveTextContent('✗')
    expect(screen.queryByRole('img', { name: /in progress/i })).not.toBeInTheDocument()
  })
})
