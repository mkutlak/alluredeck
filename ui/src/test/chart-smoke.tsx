import React from 'react'
import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'

/**
 * Parameterized smoke-test factory for recharts-based chart components.
 *
 * Every chart in the codebase satisfies the same two invariants:
 *   1. Renders a [data-chart] element (via ChartContainer) when given data.
 *   2. Does not throw when given an empty data array.
 *
 * Prerequisites — call these at the TOP of the test file, before this
 * factory, so vi.mock() hoisting works correctly:
 *
 *   import { mockRecharts } from '@/test/mocks/recharts'
 *   mockRecharts()
 *
 * Usage:
 *
 *   import { describeChartSmoke } from '@/test/chart-smoke'
 *   import { StatusTrendChart } from '../StatusTrendChart'
 *   import type { StatusTrendPoint } from '@/lib/chart-utils'
 *
 *   mockRecharts()
 *
 *   const sample: StatusTrendPoint[] = [
 *     { name: '#1', passed: 10, failed: 2, broken: 1, skipped: 0 },
 *   ]
 *
 *   describeChartSmoke('StatusTrendChart', {
 *     renderWithData: () => <StatusTrendChart data={sample} />,
 *     renderEmpty:    () => <StatusTrendChart data={[]} />,
 *   })
 *
 * Add component-specific tests in a separate describe block after this call.
 */
export interface ChartSmokeOptions {
  /** Render call that passes at least one data point. */
  renderWithData: () => React.ReactElement
  /** Render call that passes an empty data array. */
  renderEmpty: () => React.ReactElement
}

export function describeChartSmoke(componentName: string, opts: ChartSmokeOptions): void {
  describe(`${componentName} — smoke`, () => {
    it('renders a [data-chart] element when given data', () => {
      render(opts.renderWithData())
      expect(document.querySelector('[data-chart]')).not.toBeNull()
    })

    it('renders without crashing when data is empty', () => {
      render(opts.renderEmpty())
    })
  })
}
