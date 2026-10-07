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

  // Rated from counts, floored: pass_rate 100 must not hide a failure in 2,500.
  it.each([
    { name: 'passed count', suite: { total: 2500, passed: 2499, failed: 1 }, text: '99.9%' },
    // No `passed` from the API: total - failed - skipped (broken is inside failed).
    { name: 'derived passed', suite: { total: 100, failed: 43 }, text: '57.0%' },
    {
      name: 'derived passed, skipped excluded',
      suite: { total: 10, skipped: 5, pass_rate: 50 },
      text: '100%',
    },
  ])('rates the suite from counts ($name)', ({ suite, text }) => {
    renderWithProviders(<SuiteBadge suite={makeSuite({ pass_rate: 100, ...suite })} />)
    expect(screen.getByText(new RegExp(text.replace('.', '\\.')))).toBeInTheDocument()
  })

  // A suite where nothing ran is neutral: no rate, no ✓/✗, and not the
  // colourful badge classes getPassRateBadgeClass(0) falls through from.
  it('renders a skipped suite neutral with "—" for the rate', () => {
    renderWithProviders(
      <SuiteBadge
        suite={makeSuite({ total: 5, skipped: 5, passed: 0, pass_rate: 0, status: 'skipped' })}
      />,
    )
    const badge = screen.getByText(/—/)
    expect(badge).toHaveClass('text-[#6c6f85]')
    expect(badge.className).not.toMatch(/bg-primary|bg-destructive/)
    expect(badge).not.toHaveTextContent(/[✓✗⚠]/)
  })
})
