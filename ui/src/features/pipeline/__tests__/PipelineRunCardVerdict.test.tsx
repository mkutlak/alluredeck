import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { PipelineRunCard } from '../PipelineRunCard'
import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import type { PipelineRun, PipelineSuite } from '@/types/api'

function makeSuite(project_id: number, slug: string, failed = 0): PipelineSuite {
  return {
    project_id,
    slug,
    build_number: 5,
    build_id: 100 + project_id,
    pass_rate: 100,
    total: 100,
    failed,
    duration_ms: 15000,
    status: failed > 0 ? 'degraded' : 'passed',
  }
}

function makeRun(failed = 0): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    suites: [makeSuite(1, 'api-cloud'), makeSuite(2, 'ui-tests', failed)],
    aggregate: {
      suites_passed: failed > 0 ? 1 : 2,
      suites_total: 2,
      tests_passed: 200 - failed,
      tests_total: 200,
      pass_rate: ((200 - failed) / 200) * 100,
      total_duration_ms: 45000,
    },
  }
}

// The card's rate is a fact, like the feed row's: the glyph and the red failed-test
// count carry the verdict, so an amber 80% no longer has to say it alone.
describe('PipelineRunCard verdict', () => {
  it.each([
    { failed: 1, text: '1 failed test' },
    { failed: 40, text: '40 failed tests' },
  ])('leads with ✗ and counts "$text" in red when tests failed', ({ failed, text }) => {
    renderWithProviders(<PipelineRunCard run={makeRun(failed)} />)

    expect(screen.getByText('✗')).toBeInTheDocument()
    expect(screen.queryByText('✓')).not.toBeInTheDocument()
    expect(screen.getByText(text)).toHaveClass(
      'font-medium',
      ...STATUS_TEXT_CLASSES.failed.split(' '),
    )
  })

  it('keeps the rate in plain fact ink next to the failed count, never amber or red', () => {
    // 80% is amber as a pass-rate colour, so this is what proves the ink is plain.
    renderWithProviders(<PipelineRunCard run={makeRun(40)} />)

    expect(screen.getByText('80.0% overall')).toHaveClass('text-fact')
    expect(screen.getByText('1/2 suites passing')).toHaveClass('text-fact')
    expect(screen.getByText('80.0% overall').className).not.toMatch(/d20f39|fe640b|40a02b/)
  })

  it('leads with ✓ and rates a passing run in plain fact ink, with no green and no failed count', () => {
    renderWithProviders(<PipelineRunCard run={makeRun()} />)

    expect(screen.getByText('✓')).toBeInTheDocument()
    expect(screen.queryByText('✗')).not.toBeInTheDocument()
    expect(screen.getByText('100% overall')).toHaveClass('text-fact')
    expect(screen.getByText('100% overall').className).not.toMatch(/40a02b|a6e3a1/)
    expect(screen.getByText('2/2 suites passing').className).not.toMatch(/40a02b|a6e3a1/)
    expect(screen.queryByText(/failed test/)).not.toBeInTheDocument()
  })
})
