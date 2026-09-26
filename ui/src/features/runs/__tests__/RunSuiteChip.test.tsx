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

describe('RunSuiteChip', () => {
  it.each([
    { suite: makeSuite(), label: 'api-cloud' },
    { suite: makeSuite({ display_name: 'API Cloud' }), label: 'API Cloud' },
  ])('labels the chip $label, preferring display_name over slug', ({ suite, label }) => {
    renderWithProviders(<RunSuiteChip suite={suite} />)
    expect(screen.getByText(label)).toBeInTheDocument()
    expect(screen.queryByText('api-cloud') !== null).toBe(label === 'api-cloud')
  })

  it.each([4, 0])('shows a failed-count badge only for failures > 0 (%i)', (failed) => {
    renderWithProviders(<RunSuiteChip suite={makeSuite({ failed, status: 'degraded' })} />)
    expect(screen.queryByText(String(failed)) !== null).toBe(failed > 0)
  })

  it.each([
    {
      name: 'a single build links to its report',
      suite: makeSuite({
        project_id: 2,
        build_number: 9,
        builds: [{ build_id: 1, build_number: 9 }],
      }),
      href: '/projects/2/reports/9',
      shards: null,
    },
    {
      // No single report represents a suite that three shards uploaded to, so
      // linking at one of them would hide the other two.
      name: 'a sharded suite links to the project',
      suite: makeSuite({
        project_id: 84,
        slug: 'ui-users',
        failed: 4,
        status: 'degraded',
        build_number: 656,
        builds: [
          { build_id: 17463, build_number: 654 },
          { build_id: 17464, build_number: 655 },
          { build_id: 17465, build_number: 656 },
        ],
      }),
      href: '/projects/84',
      shards: '3',
    },
  ])('$name by numeric project_id, marking shards', ({ suite, href, shards }) => {
    renderWithProviders(<RunSuiteChip suite={suite} />)
    expect(screen.getByTestId('run-suite-chip')).toHaveAttribute('href', href)
    expect(screen.queryByTestId('run-suite-chip-shards')?.textContent ?? null).toBe(shards)
  })
})
