import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GanttTooltip } from '../GanttTooltip'

const makeTC = (overrides = {}) => ({
  name: 'Login test',
  full_name: 'auth.LoginTest.login',
  status: 'passed',
  start: 0,
  stop: 5000,
  duration: 5000,
  thread: 'worker-1',
  host: 'node-a',
  ...overrides,
})

const defaultColors = {
  passed: '#40a02b',
  failed: '#d20f39',
  broken: '#fe640b',
  skipped: '#8c8fa1',
}

describe('GanttTooltip', () => {
  it('renders nothing when testCase is null', () => {
    const { container } = render(
      <GanttTooltip testCase={null} position={{ x: 10, y: 20 }} statusColors={defaultColors} />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when position is null', () => {
    const { container } = render(
      <GanttTooltip testCase={makeTC()} position={null} statusColors={defaultColors} />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('shows test name when testCase and position provided', () => {
    render(
      <GanttTooltip
        testCase={makeTC()}
        position={{ x: 10, y: 20 }}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByText('Login test')).toBeInTheDocument()
  })

  it('shows status badge with correct text', () => {
    render(
      <GanttTooltip
        testCase={makeTC({ status: 'failed' })}
        position={{ x: 10, y: 20 }}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByText('failed')).toBeInTheDocument()
  })

  it('shows formatted duration', () => {
    render(
      <GanttTooltip
        testCase={makeTC({ duration: 5000 })}
        position={{ x: 10, y: 20 }}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByText('5s')).toBeInTheDocument()
  })

  it('shows worker/thread info when available', () => {
    render(
      <GanttTooltip
        testCase={makeTC({ thread: 'worker-1' })}
        position={{ x: 10, y: 20 }}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByText('Worker: worker-1')).toBeInTheDocument()
  })

  it('shows host info when available', () => {
    render(
      <GanttTooltip
        testCase={makeTC({ host: 'node-a' })}
        position={{ x: 10, y: 20 }}
        statusColors={defaultColors}
      />,
    )
    expect(screen.getByText('Host: node-a')).toBeInTheDocument()
  })

  it('does not show worker label when thread is empty', () => {
    render(
      <GanttTooltip
        testCase={makeTC({ thread: '' })}
        position={{ x: 10, y: 20 }}
        statusColors={defaultColors}
      />,
    )
    expect(screen.queryByText(/Worker:/)).toBeNull()
  })

  it('positions via inline style (left, top)', () => {
    render(
      <GanttTooltip
        testCase={makeTC()}
        position={{ x: 42, y: 84 }}
        statusColors={defaultColors}
      />,
    )
    const tooltip = screen.getByTestId('gantt-tooltip')
    expect(tooltip).toHaveStyle({ left: '42px', top: '84px' })
  })
})
