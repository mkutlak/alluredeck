import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { SuiteBadge } from '../SuiteBadge'
import type { PipelineSuite } from '@/types/api'

function makeSuite(overrides?: Partial<PipelineSuite>): PipelineSuite {
  return {
    project_id: 'api-cloud',
    build_order: 5,
    pass_rate: 100,
    total: 42,
    failed: 0,
    duration_ms: 15000,
    status: 'passed',
    ...overrides,
  }
}

describe('SuiteBadge', () => {
  it('renders suite name and pass rate', () => {
    renderWithProviders(<SuiteBadge suite={makeSuite()} />)
    expect(screen.getByText('api-cloud')).toBeInTheDocument()
    expect(screen.getByText(/100%/)).toBeInTheDocument()
  })

  it('links to the correct project URL', () => {
    renderWithProviders(<SuiteBadge suite={makeSuite({ project_id: 'ui-tests' })} />)
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/projects/ui-tests')
  })

  it('shows failed count when present', () => {
    renderWithProviders(<SuiteBadge suite={makeSuite({ failed: 3, pass_rate: 71, status: 'degraded' })} />)
    expect(screen.getByText('3 failed')).toBeInTheDocument()
  })

  it('shows correct status icon for failed suite', () => {
    renderWithProviders(<SuiteBadge suite={makeSuite({ pass_rate: 50, status: 'failed' })} />)
    expect(screen.getByText(/✗/)).toBeInTheDocument()
  })
})
