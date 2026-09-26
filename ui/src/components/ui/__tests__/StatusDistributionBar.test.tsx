import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { TooltipProvider } from '@/components/ui/tooltip'
import { StatusDistributionBar } from '../StatusDistributionBar'

describe('StatusDistributionBar', () => {
  it.each([
    [
      { passed: 35, failed: 1, broken: 0, skipped: 2 },
      '35 passed, 1 failed, 0 broken, 2 skipped',
      ['passed', 'failed', 'skipped'],
    ],
    [
      { passed: 10, failed: 0, broken: 0, skipped: 0 },
      '10 passed, 0 failed, 0 broken, 0 skipped',
      ['passed'],
    ],
  ])('labels all counts and draws a segment per non-zero status: %o', (counts, label, statuses) => {
    render(
      <TooltipProvider>
        <StatusDistributionBar {...counts} />
      </TooltipProvider>,
    )
    const segments = within(screen.getByRole('img', { name: label })).getAllByTestId(
      'status-segment',
    )
    expect(segments.map((s) => s.dataset.status)).toEqual(statuses)
  })
})
