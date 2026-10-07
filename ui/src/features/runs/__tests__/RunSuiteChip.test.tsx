import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

  // "Degraded" is derived from pass rate, so a suite at 95% was drawn as an
  // amber warning although tests in it failed. Any failure reads as ✗.
  it.each(['degraded', 'failed'] as const)(
    'marks a suite with failures ✗ whatever its pass-rate status (%s)',
    (status) => {
      renderWithProviders(<RunSuiteChip suite={makeSuite({ failed: 3, status })} />)
      const chip = screen.getByTestId('run-suite-chip')
      expect(chip).toHaveTextContent('✗')
      expect(chip).not.toHaveTextContent('⚠')
    },
  )

  // Nothing ran in the suite: neither the green ✓ nor the red ✗ is true.
  it('draws a skipped suite neutral: no ✓, ✗ or ⚠, in the skipped gray', () => {
    renderWithProviders(
      <RunSuiteChip suite={makeSuite({ total: 5, skipped: 5, passed: 0, status: 'skipped' })} />,
    )
    const chip = screen.getByTestId('run-suite-chip')
    expect(chip).not.toHaveTextContent(/[✓✗⚠]/)
    expect(screen.getByText('○')).toHaveClass('text-[#6c6f85]')
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

  // Opening upward covered the run's own ID and branch on line 1.
  it('opens the shard tooltip below the chip', async () => {
    const user = userEvent.setup()
    const builds = [1, 2, 3].map((n) => ({ build_id: 100 + n, build_number: n }))
    renderWithProviders(<RunSuiteChip suite={makeSuite({ failed: 4, builds })} />)

    await user.tab() // keyboard focus opens a Radix tooltip without the hover delay
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('api-cloud: 4 failed across 3 shards')
    expect(tooltip).toHaveAttribute('data-side', 'bottom')
  })
})
