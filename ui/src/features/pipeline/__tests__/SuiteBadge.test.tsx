import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { SuiteBadge } from '../SuiteBadge'
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

describe('SuiteBadge', () => {
  // Links use the numeric project_id, never the slug (404f3ee: slug links broke on parent pages).
  it('links to the correct project URL', () => {
    renderWithProviders(<SuiteBadge suite={makeSuite({ project_id: 2, slug: 'ui-tests' })} />)
    expect(screen.getByRole('link', { name: /ui-tests/ })).toHaveAttribute('href', '/projects/2')
  })

  it('marks a failed suite and shows its failed count', () => {
    renderWithProviders(
      <SuiteBadge suite={makeSuite({ failed: 3, pass_rate: 50, status: 'failed' })} />,
    )
    expect(screen.getByText(/✗/)).toBeInTheDocument()
    expect(screen.getByText('3 failed')).toBeInTheDocument()
  })
})
