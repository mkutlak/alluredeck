import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TimelineMinimap } from '../TimelineMinimap'

// d3 brush needs real layout; a chainable stub is enough for rendering.
vi.mock('d3-brush', () => ({
  brushX: () => {
    const brush = Object.assign(vi.fn(), { extent: () => brush, on: () => brush })
    return brush
  },
}))
vi.mock('d3-selection', () => ({
  select: () => {
    const sel: Record<string, unknown> = {}
    sel.call = sel.selectAll = sel.attr = () => sel
    return sel
  },
}))

const colors = { passed: '#40a02b', failed: '#d20f39', broken: '#fe640b', skipped: '#8c8fa1' }

describe('TimelineMinimap', () => {
  it('each bar rect has correct fill color based on status', () => {
    const testCases = ['passed', 'failed', 'broken', 'skipped'].map((status, i) => ({
      name: status,
      full_name: `suite.${status}`,
      status,
      start: i * 500,
      stop: i * 500 + 500,
      duration: 500,
      thread: '',
      host: '',
    }))
    render(
      <TimelineMinimap
        testCases={testCases}
        minStart={0}
        maxStop={2000}
        statusColors={colors}
        width={400}
        onBrushChange={vi.fn()}
        viewportRange={null}
      />,
    )

    expect(screen.getAllByTestId('minimap-bar').map((bar) => bar.getAttribute('fill'))).toEqual([
      '#40a02b',
      '#d20f39',
      '#fe640b',
      '#8c8fa1',
    ])
  })
})
