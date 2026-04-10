import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import { PipelineRunCard } from '../PipelineRunCard'
import type { PipelineRun } from '@/types/api'

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    commit_sha: 'abc1234def5678',
    branch: 'main',
    ci_build_url: 'https://ci.example.com/pipelines/123',
    timestamp: '2026-04-03T18:00:00Z',
    suites: [
      {
        project_id: 1,
        slug: 'api-cloud',
        build_order: 5,
        pass_rate: 100,
        total: 42,
        failed: 0,
        duration_ms: 15000,
        status: 'passed',
      },
      {
        project_id: 2,
        slug: 'ui-tests',
        build_order: 3,
        pass_rate: 85,
        total: 100,
        failed: 15,
        duration_ms: 30000,
        status: 'degraded',
      },
    ],
    aggregate: {
      suites_passed: 1,
      suites_total: 2,
      tests_passed: 127,
      tests_total: 142,
      pass_rate: 89.4,
      total_duration_ms: 45000,
    },
    ...overrides,
  }
}

describe('PipelineRunCard', () => {
  it('renders truncated SHA and branch badge', () => {
    renderWithProviders(<PipelineRunCard run={makeRun()} />)
    expect(screen.getByText('abc1234')).toBeInTheDocument()
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('renders aggregate summary', () => {
    renderWithProviders(<PipelineRunCard run={makeRun()} />)
    expect(screen.getByText(/1\/2 suites passing/)).toBeInTheDocument()
    expect(screen.getByText(/89.4% overall/)).toBeInTheDocument()
  })

  it('expands to show suite grid on click', async () => {
    const user = userEvent.setup()
    renderWithProviders(<PipelineRunCard run={makeRun()} />)

    // Suite names should not be visible initially
    expect(screen.queryByText('api-cloud')).not.toBeInTheDocument()

    // Click to expand
    await user.click(screen.getByRole('button'))

    // Suite names should now be visible
    expect(screen.getByText('api-cloud')).toBeInTheDocument()
    expect(screen.getByText('ui-tests')).toBeInTheDocument()
  })

  it('links SHA to CI build URL', () => {
    renderWithProviders(<PipelineRunCard run={makeRun()} />)
    const link = screen.getByRole('link', { name: /abc1234/ })
    expect(link).toHaveAttribute('href', 'https://ci.example.com/pipelines/123')
  })
})
