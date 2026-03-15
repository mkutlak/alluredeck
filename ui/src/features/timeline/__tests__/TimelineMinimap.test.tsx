import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TimelineMinimap } from '../TimelineMinimap'

// Mock D3 modules since they don't work in jsdom
vi.mock('d3-brush', () => ({
  brushX: vi.fn(() => {
    const brush = Object.assign(vi.fn(), {} as Record<string, unknown>)
    brush.extent = vi.fn(() => brush)
    brush.on = vi.fn(() => brush)
    brush.move = vi.fn(() => brush)
    return brush
  }),
}))

vi.mock('d3-selection', () => ({
  select: vi.fn(() => {
    const sel: Record<string, unknown> = {}
    sel.call = vi.fn(() => sel)
    sel.select = vi.fn(() => sel)
    sel.selectAll = vi.fn(() => sel)
    sel.attr = vi.fn(() => sel)
    sel.style = vi.fn(() => sel)
    return sel
  }),
}))

const makeTC = (overrides = {}) => ({
  name: 'test',
  full_name: 'suite.test',
  status: 'passed' as string,
  start: 0,
  stop: 1000,
  duration: 1000,
  thread: '',
  host: '',
  ...overrides,
})

const defaultColors = {
  passed: '#40a02b',
  failed: '#d20f39',
  broken: '#fe640b',
  skipped: '#8c8fa1',
}

describe('TimelineMinimap', () => {
  it('renders an SVG element with data-testid="timeline-minimap"', () => {
    render(
      <TimelineMinimap
        testCases={[makeTC()]}
        minStart={0}
        maxStop={1000}
        statusColors={defaultColors}
        width={400}
        onBrushChange={vi.fn()}
        viewportRange={null}
      />,
    )
    expect(screen.getByTestId('timeline-minimap')).toBeInTheDocument()
  })

  it('SVG has correct width and height (height=40)', () => {
    render(
      <TimelineMinimap
        testCases={[makeTC()]}
        minStart={0}
        maxStop={1000}
        statusColors={defaultColors}
        width={600}
        onBrushChange={vi.fn()}
        viewportRange={null}
      />,
    )
    const svg = screen.getByTestId('timeline-minimap')
    expect(svg).toHaveAttribute('width', '600')
    expect(svg).toHaveAttribute('height', '40')
  })

  it('renders a rect for each test case (data-testid="minimap-bar")', () => {
    const testCases = [
      makeTC({ name: 'tc1', start: 0, stop: 500 }),
      makeTC({ name: 'tc2', start: 500, stop: 1000 }),
      makeTC({ name: 'tc3', start: 1000, stop: 1500 }),
    ]
    render(
      <TimelineMinimap
        testCases={testCases}
        minStart={0}
        maxStop={1500}
        statusColors={defaultColors}
        width={400}
        onBrushChange={vi.fn()}
        viewportRange={null}
      />,
    )
    const bars = screen.getAllByTestId('minimap-bar')
    expect(bars).toHaveLength(3)
  })

  it('each bar rect has correct fill color based on status', () => {
    const testCases = [
      makeTC({ name: 'p', status: 'passed', start: 0, stop: 500 }),
      makeTC({ name: 'f', status: 'failed', start: 500, stop: 1000 }),
      makeTC({ name: 'b', status: 'broken', start: 1000, stop: 1500 }),
      makeTC({ name: 's', status: 'skipped', start: 1500, stop: 2000 }),
    ]
    render(
      <TimelineMinimap
        testCases={testCases}
        minStart={0}
        maxStop={2000}
        statusColors={defaultColors}
        width={400}
        onBrushChange={vi.fn()}
        viewportRange={null}
      />,
    )
    const bars = screen.getAllByTestId('minimap-bar')
    // bars are sorted by start so order matches input
    expect(bars[0]).toHaveAttribute('fill', '#40a02b')
    expect(bars[1]).toHaveAttribute('fill', '#d20f39')
    expect(bars[2]).toHaveAttribute('fill', '#fe640b')
    expect(bars[3]).toHaveAttribute('fill', '#8c8fa1')
  })

  it('has a brush group element (data-testid="minimap-brush")', () => {
    render(
      <TimelineMinimap
        testCases={[makeTC()]}
        minStart={0}
        maxStop={1000}
        statusColors={defaultColors}
        width={400}
        onBrushChange={vi.fn()}
        viewportRange={null}
      />,
    )
    expect(screen.getByTestId('minimap-brush')).toBeInTheDocument()
  })

  it('renders nothing meaningful with empty testCases array (still renders SVG container)', () => {
    render(
      <TimelineMinimap
        testCases={[]}
        minStart={0}
        maxStop={1000}
        statusColors={defaultColors}
        width={400}
        onBrushChange={vi.fn()}
        viewportRange={null}
      />,
    )
    expect(screen.getByTestId('timeline-minimap')).toBeInTheDocument()
    expect(screen.queryAllByTestId('minimap-bar')).toHaveLength(0)
  })
})
