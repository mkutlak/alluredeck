import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TimelineLegend } from '../TimelineLegend'

const colors = { passed: '#40a02b', failed: '#d20f39', broken: '#fe640b', skipped: '#8c8fa1' }

function makeTC(status: string) {
  return {
    name: `test-${status}`,
    full_name: `suite.test-${status}`,
    status,
    start: 0,
    stop: 1000,
    duration: 1000,
    thread: 'worker-1',
    host: 'node-a',
  }
}

describe('TimelineLegend', () => {
  it('lists only the statuses present, each with its status color', () => {
    render(
      <TimelineLegend
        testCases={[makeTC('failed'), makeTC('passed'), makeTC('failed')]}
        statusColors={colors}
      />,
    )

    expect(screen.getByTestId('legend').textContent).toBe('PassedFailed')
    expect(screen.getByTestId('legend-swatch-passed')).toHaveStyle({ backgroundColor: '#40a02b' })
    expect(screen.getByTestId('legend-swatch-failed')).toHaveStyle({ backgroundColor: '#d20f39' })
  })
})
