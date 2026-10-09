import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { RunSuiteChip } from '../RunSuiteChip'
import type { PipelineSuite } from '@/types/api'

function makeSuite(overrides?: Partial<PipelineSuite>): PipelineSuite {
  return {
    project_id: 1,
    slug: 'api-cloud',
    build_number: 5,
    build_id: 105,
    pass_rate: 100,
    total: 42,
    failed: 0,
    duration_ms: 15000,
    status: 'passed',
    builds: [{ build_id: 105, build_number: 5 }],
    ...overrides,
  }
}

const shards = [1, 2, 3].map((n) => ({ build_id: 100 + n, build_number: n }))

// Without a name the chip reads "api-cloud 4 3": the glyph is hidden and the counts
// have no words. The name says what the counts mean.
describe('RunSuiteChip name', () => {
  it.each([
    {
      name: 'sharded',
      suite: makeSuite({ failed: 4, status: 'degraded', builds: shards }),
      want: 'api-cloud: 4 failed across 3 shards',
    },
    {
      name: 'failing',
      suite: makeSuite({ failed: 3, status: 'degraded' }),
      want: 'api-cloud: 3 failed',
    },
  ])('names a $name chip "$want"', ({ suite, want }) => {
    renderWithProviders(<RunSuiteChip suite={suite} />)
    expect(screen.getByRole('link', { name: want })).toBeInTheDocument()
  })
})
