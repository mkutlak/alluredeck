import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { RunSuiteChip } from '../RunSuiteChip'
import type { PipelineSuite } from '@/types/api'

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
    builds: [{ build_id: 105, build_number: 5 }],
    ...overrides,
  }
}

describe('RunSuiteChip', () => {
  it('renders the suite slug', () => {
    renderWithProviders(<RunSuiteChip suite={makeSuite()} />)
    expect(screen.getByText('api-cloud')).toBeInTheDocument()
  })

  it('prefers display_name over slug', () => {
    renderWithProviders(<RunSuiteChip suite={makeSuite({ display_name: 'API Cloud' })} />)
    expect(screen.getByText('API Cloud')).toBeInTheDocument()
    expect(screen.queryByText('api-cloud')).not.toBeInTheDocument()
  })

  it('links to the report using numeric project_id and build_number', () => {
    renderWithProviders(
      <RunSuiteChip
        suite={makeSuite({ project_id: 2, build_number: 9, builds: [{ build_id: 1, build_number: 9 }] })}
      />,
    )
    expect(screen.getByTestId('run-suite-chip')).toHaveAttribute('href', '/projects/2/reports/9')
  })

  it('shows the failed count when there are failures', () => {
    renderWithProviders(<RunSuiteChip suite={makeSuite({ failed: 4, status: 'degraded' })} />)
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('does not show a failed count badge when failed is 0', () => {
    renderWithProviders(<RunSuiteChip suite={makeSuite({ failed: 0 })} />)
    expect(screen.queryByText('0')).not.toBeInTheDocument()
  })

  it('has data-testid="run-suite-chip"', () => {
    renderWithProviders(<RunSuiteChip suite={makeSuite()} />)
    expect(screen.getByTestId('run-suite-chip')).toBeInTheDocument()
  })

  describe('sharded suites', () => {
    const sharded = makeSuite({
      project_id: 84,
      slug: 'ui-users',
      failed: 4,
      status: 'degraded',
      build_number: 656,
      builds: [
        { build_id: 17463, build_number: 654 },
        { build_id: 17464, build_number: 655 },
        { build_id: 17465, build_number: 656 },
      ],
    })

    it('marks how many builds contributed', () => {
      renderWithProviders(<RunSuiteChip suite={sharded} />)
      const marker = screen.getByTestId('run-suite-chip-shards')
      expect(marker).toHaveTextContent('3')
    })

    // No single report represents a suite that three shards uploaded to, so
    // linking at one of them would hide the other two.
    it('links to the project rather than one arbitrary shard report', () => {
      renderWithProviders(<RunSuiteChip suite={sharded} />)
      expect(screen.getByTestId('run-suite-chip')).toHaveAttribute('href', '/projects/84')
    })

    it('does not show a shard marker for a single-build suite', () => {
      renderWithProviders(<RunSuiteChip suite={makeSuite()} />)
      expect(screen.queryByTestId('run-suite-chip-shards')).not.toBeInTheDocument()
    })
  })
})
