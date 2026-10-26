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

// Two suites arrived; `awaiting` more are expected.
function makeRun(awaiting?: number, failed = 0): PipelineRun {
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
      awaiting_suites: awaiting,
    },
  }
}

const skippedInk = STATUS_TEXT_CLASSES.skipped.split(' ')

describe('PipelineRunCard for a pipeline run that is still uploading', () => {
  it.each([
    { awaiting: 3, text: 'awaiting 3 suites' },
    { awaiting: 1, text: 'awaiting 1 suite' },
  ])('reads $text in neutral ink instead of "x/y suites passing"', ({ awaiting, text }) => {
    renderWithProviders(<PipelineRunCard run={makeRun(awaiting)} />)

    expect(screen.queryByText(/suites passing/)).not.toBeInTheDocument()
    expect(screen.getByText(text)).toHaveClass(...skippedInk)
  })

  it('rates the arrived tests in neutral ink, never green or amber', () => {
    renderWithProviders(<PipelineRunCard run={makeRun(2)} />)
    expect(screen.getByText('100% overall')).toHaveClass(...skippedInk)
    expect(screen.getByText('100% overall').className).not.toMatch(/40a02b|a6e3a1/)
  })

  it('stays neutral when something failed, and keeps the rate and duration', () => {
    // 80% would be amber on a finished run, so this is what proves the ink is neutral.
    renderWithProviders(<PipelineRunCard run={makeRun(2, 40)} />)

    expect(screen.getByText('80.0% overall')).toHaveClass(...skippedInk)
    expect(screen.getByText('80.0% overall').className).not.toMatch(/d20f39|fe640b|40a02b/)
    expect(screen.getByText('awaiting 2 suites')).toHaveClass(...skippedInk)
    expect(screen.getByText(/45s/)).toBeInTheDocument()
  })

  it.each([undefined, 0])('keeps "x/y suites passing" when awaiting_suites is %s', (awaiting) => {
    renderWithProviders(<PipelineRunCard run={makeRun(awaiting)} />)

    expect(screen.getByText('2/2 suites passing')).toHaveClass('text-fact')
    expect(screen.queryByText(/awaiting/)).not.toBeInTheDocument()
  })
})
