import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TimelineLegend } from '../TimelineLegend'

const defaultColors = {
  passed: '#40a02b',
  failed: '#d20f39',
  broken: '#fe640b',
  skipped: '#8c8fa1',
}

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
  it('has data-testid="legend"', () => {
    render(<TimelineLegend testCases={[makeTC('passed')]} statusColors={defaultColors} />)
    expect(screen.getByTestId('legend')).toBeInTheDocument()
  })

  it('shows only statuses present in testCases (passed+failed, no broken/skipped)', () => {
    render(
      <TimelineLegend
        testCases={[makeTC('passed'), makeTC('failed')]}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByText('Passed')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.queryByText('Broken')).toBeNull()
    expect(screen.queryByText('Skipped')).toBeNull()
  })

  it('does not show statuses not present in data', () => {
    render(<TimelineLegend testCases={[makeTC('passed')]} statusColors={defaultColors} />)
    expect(screen.queryByText('Failed')).toBeNull()
    expect(screen.queryByText('Broken')).toBeNull()
    expect(screen.queryByText('Skipped')).toBeNull()
  })

  it('shows all 4 statuses when all present', () => {
    render(
      <TimelineLegend
        testCases={[makeTC('passed'), makeTC('failed'), makeTC('broken'), makeTC('skipped')]}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByText('Passed')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('Broken')).toBeInTheDocument()
    expect(screen.getByText('Skipped')).toBeInTheDocument()
  })

  it('uses correct color for each status swatch', () => {
    render(
      <TimelineLegend
        testCases={[makeTC('passed'), makeTC('failed'), makeTC('broken'), makeTC('skipped')]}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByTestId('legend-swatch-passed')).toHaveStyle({
      backgroundColor: '#40a02b',
    })
    expect(screen.getByTestId('legend-swatch-failed')).toHaveStyle({
      backgroundColor: '#d20f39',
    })
    expect(screen.getByTestId('legend-swatch-broken')).toHaveStyle({
      backgroundColor: '#fe640b',
    })
    expect(screen.getByTestId('legend-swatch-skipped')).toHaveStyle({
      backgroundColor: '#8c8fa1',
    })
  })
})
