import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import { PipelineRunCard } from '../PipelineRunCard'
import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import type { PipelineRun, PipelineSuite } from '@/types/api'

function makeSuite(project_id: number, slug: string): PipelineSuite {
  return {
    project_id,
    slug,
    build_number: 5,
    build_id: 100 + project_id,
    pass_rate: 100,
    total: 42,
    failed: 0,
    duration_ms: 15000,
    status: 'passed',
  }
}

const run: PipelineRun = {
  commit_sha: 'abc1234def5678',
  branch: 'main',
  ci_build_url: 'https://ci.example.com/pipelines/123',
  timestamp: '2026-04-03T18:00:00Z',
  suites: [makeSuite(1, 'api-cloud'), makeSuite(2, 'ui-tests')],
  aggregate: {
    suites_passed: 1,
    suites_total: 2,
    tests_passed: 127,
    tests_total: 142,
    pass_rate: 89.4,
    total_duration_ms: 45000,
  },
}

describe('PipelineRunCard', () => {
  it('links the short SHA to the CI build and shows the branch', () => {
    renderWithProviders(<PipelineRunCard run={run} />)
    expect(screen.getByRole('link', { name: 'abc1234' })).toHaveAttribute(
      'href',
      'https://ci.example.com/pipelines/123',
    )
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('rates the run from counts, floored, not from pass_rate', () => {
    renderWithProviders(
      <PipelineRunCard
        run={{
          ...run,
          aggregate: { ...run.aggregate, tests_passed: 2499, tests_total: 2500, pass_rate: 100 },
        }}
      />,
    )
    expect(screen.getByText('99.9% overall')).toBeInTheDocument()
  })

  it('shows "—" in a neutral colour when every suite was skipped', () => {
    renderWithProviders(
      <PipelineRunCard
        run={{
          ...run,
          suites: [{ ...makeSuite(1, 'api-cloud'), total: 5, skipped: 5, status: 'skipped' }],
          aggregate: {
            ...run.aggregate,
            tests_passed: 0,
            tests_total: 5,
            tests_skipped: 5,
            pass_rate: 0,
          },
        }}
      />,
    )
    expect(screen.getByText('— overall')).toHaveClass(...STATUS_TEXT_CLASSES.skipped.split(' '))
  })

  // Zero counts alone do not mean "skipped": a suite whose stats could not be
  // read is `failed` and keeps the failing colour.
  it('keeps the failing colour for a suite with no readable stats', () => {
    renderWithProviders(
      <PipelineRunCard
        run={{
          ...run,
          suites: [{ ...makeSuite(1, 'api-cloud'), total: 0, status: 'failed', pass_rate: 0 }],
          aggregate: { ...run.aggregate, tests_passed: 0, tests_total: 0, pass_rate: 0 },
        }}
      />,
    )
    expect(screen.getByText('— overall').className).toMatch(/d20f39/)
  })

  it('expands to show suite grid on click', async () => {
    const user = userEvent.setup()
    renderWithProviders(<PipelineRunCard run={run} />)
    expect(screen.queryByText('api-cloud')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button'))

    expect(screen.getByText('api-cloud')).toBeInTheDocument()
    expect(screen.getByText('ui-tests')).toBeInTheDocument()
  })
})
