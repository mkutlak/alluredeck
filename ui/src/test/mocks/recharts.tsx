import React from 'react'
import { vi } from 'vitest'

/**
 * Shared mock for recharts.
 *
 * Call this at module scope in test files that render recharts-based chart
 * components:
 *
 *   import { mockRecharts } from '@/test/mocks/recharts'
 *   mockRecharts()
 *
 * ResponsiveContainer requires a real browser layout engine to compute
 * dimensions; jsdom provides none. This mock replaces it with a plain div
 * of fixed size so all child chart elements still render and can be
 * asserted against. All other recharts exports are passed through unchanged
 * (via importActual) so chart-specific behaviour is preserved.
 */
export function mockRecharts(): void {
  vi.mock('recharts', async () => {
    const actual = await vi.importActual<typeof import('recharts')>('recharts')
    return {
      ...actual,
      ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
        <div style={{ width: 500, height: 300 }}>{children}</div>
      ),
    }
  })
}
