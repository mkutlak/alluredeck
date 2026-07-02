import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TooltipProvider } from '@/components/ui/tooltip'
import { StatusDistributionBar } from '../StatusDistributionBar'

function renderBar(props: { passed: number; failed: number; broken: number; skipped: number }) {
  return render(
    <TooltipProvider>
      <StatusDistributionBar {...props} />
    </TooltipProvider>,
  )
}

describe('StatusDistributionBar', () => {
  it('renders an aria-label summarizing all counts', () => {
    renderBar({ passed: 35, failed: 1, broken: 0, skipped: 2 })
    expect(screen.getByLabelText('35 passed, 1 failed, 0 broken, 2 skipped')).toBeInTheDocument()
  })

  it('renders a segment for each non-zero status', () => {
    renderBar({ passed: 10, failed: 2, broken: 1, skipped: 0 })
    // 3 non-zero segments (skipped is 0, so omitted)
    expect(screen.getAllByTestId('status-segment')).toHaveLength(3)
  })

  it('omits segments for zero-count statuses', () => {
    renderBar({ passed: 10, failed: 0, broken: 0, skipped: 0 })
    const segments = screen.getAllByTestId('status-segment')
    expect(segments).toHaveLength(1)
    expect(segments[0]).toHaveAttribute('data-status', 'passed')
  })

  it('shows a tooltip with counts on hover', async () => {
    const user = userEvent.setup()
    renderBar({ passed: 35, failed: 1, broken: 0, skipped: 2 })
    const bar = screen.getByLabelText('35 passed, 1 failed, 0 broken, 2 skipped')
    await user.hover(bar)
    const matches = await screen.findAllByText(/35 passed/i)
    expect(matches.length).toBeGreaterThan(0)
  })

  it('renders nothing visible when all counts are zero', () => {
    renderBar({ passed: 0, failed: 0, broken: 0, skipped: 0 })
    expect(screen.getByLabelText('0 passed, 0 failed, 0 broken, 0 skipped')).toBeInTheDocument()
  })
})
