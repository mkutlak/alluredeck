import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TimelineTestCase } from '@/types/api'
import { TimelineChart } from '../TimelineChart'

function makeTestCase(overrides: Partial<TimelineTestCase> = {}): TimelineTestCase {
  return {
    name: 'Login test',
    full_name: 'com.example.LoginTest',
    status: 'passed',
    start: 1000000,
    stop: 1005000,
    duration: 5000,
    thread: 'main',
    host: 'host-1',
    ...overrides,
  }
}

const MIN_START = 1000000
const MAX_STOP = 1020000

describe('TimelineChart', () => {
  it('renders lane labels for thread-based strategy', () => {
    const testCases = [
      makeTestCase({ name: 'test-1', thread: 'worker-1' }),
      makeTestCase({ name: 'test-2', thread: 'worker-2' }),
    ]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByText('worker-1')).toBeInTheDocument()
    expect(screen.getByText('worker-2')).toBeInTheDocument()
  })

  it('renders "Tests" lane when no thread or host info', () => {
    const testCases = [makeTestCase({ name: 'solo', thread: '', host: '' })]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByText('Tests')).toBeInTheDocument()
  })

  it('renders a bar element for each test case', () => {
    const testCases = [
      makeTestCase({ name: 'Login test' }),
      makeTestCase({ name: 'Logout test', start: 1010000, stop: 1015000 }),
    ]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('bar-Login test')).toBeInTheDocument()
    expect(screen.getByTestId('bar-Logout test')).toBeInTheDocument()
  })

  it('applies passed status color to bar via inline style', () => {
    const testCases = [makeTestCase({ name: 'pass-test', status: 'passed' })]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    const bar = screen.getByTestId('bar-pass-test')
    expect(bar).toHaveStyle({ backgroundColor: '#40a02b' })
  })

  it('applies failed status color to bar via inline style', () => {
    const testCases = [makeTestCase({ name: 'fail-test', status: 'failed' })]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    const bar = screen.getByTestId('bar-fail-test')
    expect(bar).toHaveStyle({ backgroundColor: '#d20f39' })
  })

  it('renders time axis element', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('time-axis')).toBeInTheDocument()
  })

  it('renders +0s tick at start of time axis', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByText('+0s')).toBeInTheDocument()
  })

  it('renders legend with Passed and Failed labels', () => {
    const testCases = [
      makeTestCase({ name: 'p', status: 'passed' }),
      makeTestCase({ name: 'f', status: 'failed', start: 1010000, stop: 1015000 }),
    ]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('legend')).toBeInTheDocument()
    expect(screen.getByText('Passed')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
  })

  it('handles a single test case without crashing', () => {
    const testCases = [makeTestCase({ name: 'solo-test' })]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('bar-solo-test')).toBeInTheDocument()
  })

  it('uses host strategy when no thread info is present', () => {
    const testCases = [
      makeTestCase({ name: 'test-1', thread: '', host: 'node-1' }),
      makeTestCase({ name: 'test-2', thread: '', host: 'node-2', start: 1010000, stop: 1015000 }),
    ]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByText('node-1')).toBeInTheDocument()
    expect(screen.getByText('node-2')).toBeInTheDocument()
  })

  it('groups test cases by their lane', () => {
    const testCases = [
      makeTestCase({ name: 'a', thread: 'lane-A' }),
      makeTestCase({ name: 'b', thread: 'lane-B', start: 1010000, stop: 1015000 }),
    ]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    // Both lanes appear
    expect(screen.getByText('lane-A')).toBeInTheDocument()
    expect(screen.getByText('lane-B')).toBeInTheDocument()
    // Both bars appear
    expect(screen.getByTestId('bar-a')).toBeInTheDocument()
    expect(screen.getByTestId('bar-b')).toBeInTheDocument()
  })

  it('correctly distributes many test cases across multiple lanes via Map indexing', () => {
    const NUM_LANES = 5
    const TESTS_PER_LANE = 4
    const testCases = Array.from({ length: NUM_LANES * TESTS_PER_LANE }, (_, i) => {
      const lane = (i % NUM_LANES) + 1
      return makeTestCase({
        name: `tc-${i}`,
        thread: `worker-${lane}`,
        start: MIN_START + i * 500,
        stop: MIN_START + i * 500 + 400,
      })
    })
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MIN_START + 30000} />)
    for (let lane = 1; lane <= NUM_LANES; lane++) {
      expect(screen.getByText(`worker-${lane}`)).toBeInTheDocument()
    }
    for (let i = 0; i < NUM_LANES * TESTS_PER_LANE; i++) {
      expect(screen.getByTestId(`bar-tc-${i}`)).toBeInTheDocument()
    }
  })
})
