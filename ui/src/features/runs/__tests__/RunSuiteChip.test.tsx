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
    ...overrides,
  }
}

describe('RunSuiteChip', () => {
  it('renders the suite slug', () => {
    renderWithProviders(<RunSuiteChip suite={makeSuite()} />)
    expect(screen.getByText('api-cloud')).toBeInTheDocument()
  })

  it('links to the report using numeric project_id and build_number', () => {
    renderWithProviders(<RunSuiteChip suite={makeSuite({ project_id: 2, build_number: 9 })} />)
    const link = screen.getByTestId('run-suite-chip')
    expect(link).toHaveAttribute('href', '/projects/2/reports/9')
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
})
