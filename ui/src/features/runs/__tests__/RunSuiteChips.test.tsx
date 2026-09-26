import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import { RunSuiteChips } from '../RunSuiteChips'
import type { PipelineSuite } from '@/types/api'

function makeSuite(overrides: Partial<PipelineSuite> & { project_id: number }): PipelineSuite {
  return {
    slug: `suite-${overrides.project_id}`,
    build_number: 1,
    build_id: 1,
    pass_rate: 100,
    total: 10,
    failed: 0,
    duration_ms: 1000,
    status: 'passed',
    builds: [{ build_id: 1, build_number: 1 }],
    ...overrides,
  }
}

const failing = makeSuite({ project_id: 1, failed: 3, status: 'degraded' })

describe('RunSuiteChips', () => {
  // A run of twenty suites where most passed cost three wrapped lines of chips.
  it.each([
    {
      name: 'with two passing',
      suites: [failing, makeSuite({ project_id: 2 }), makeSuite({ project_id: 3 })],
      passed: '· 2 passed',
    },
    { name: 'with none passing', suites: [failing], passed: null },
  ])('renders only the failing suites and counts the rest ($name)', ({ suites, passed }) => {
    renderWithProviders(<RunSuiteChips suites={suites} />)

    expect(screen.getAllByTestId('run-suite-chip')).toHaveLength(1)
    expect(screen.queryByText(/passed/)?.textContent ?? null).toBe(passed)
  })

  it('orders failing suites worst first', () => {
    renderWithProviders(
      <RunSuiteChips
        suites={[
          makeSuite({ project_id: 1, slug: 'few', failed: 2, status: 'degraded' }),
          makeSuite({ project_id: 2, slug: 'many', failed: 11, status: 'failed' }),
        ]}
      />,
    )

    const chips = screen.getAllByTestId('run-suite-chip')
    expect(chips[0]).toHaveTextContent('many')
    expect(chips[1]).toHaveTextContent('few')
  })

  it('caps the visible chips and reveals the rest on demand', async () => {
    const user = userEvent.setup()
    const suites = Array.from({ length: 7 }, (_, i) =>
      makeSuite({ project_id: i + 1, failed: 7 - i, status: 'degraded' }),
    )
    renderWithProviders(<RunSuiteChips suites={suites} />)

    expect(screen.getAllByTestId('run-suite-chip')).toHaveLength(4)

    await user.click(screen.getByTestId('run-suite-chips-more'))
    expect(screen.getAllByTestId('run-suite-chip')).toHaveLength(7)
  })

  it('renders nothing when every suite passed', () => {
    renderWithProviders(
      <RunSuiteChips suites={[makeSuite({ project_id: 1 }), makeSuite({ project_id: 2 })]} />,
    )
    expect(screen.queryByTestId('run-suite-chips')).not.toBeInTheDocument()
  })
})
