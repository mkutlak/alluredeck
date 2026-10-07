import { describe, it, expect } from 'vitest'

import { summarizeRun } from '../rowFormat'
import type { PipelineRun, PipelineSuite } from '@/types/api'

function suite(project_id: number, failed: number, status: PipelineSuite['status']): PipelineSuite {
  return {
    project_id,
    slug: `suite-${project_id}`,
    build_number: 1,
    build_id: 100 + project_id,
    pass_rate: 100,
    total: 10,
    failed,
    duration_ms: 1000,
    status,
  }
}

function run(suites: PipelineSuite[], awaiting_suites?: number): PipelineRun {
  return {
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: '2026-10-07T09:12:00Z',
    suites,
    aggregate: {
      suites_passed: suites.filter((s) => s.failed === 0).length,
      suites_total: suites.length,
      tests_passed: 10,
      tests_total: 20,
      pass_rate: 50,
      total_duration_ms: 2000,
      awaiting_suites,
    },
  }
}

describe('summarizeRun while suites are still arriving', () => {
  it('gives an all-passing run no verdict: the neutral ○, never ✓', () => {
    const s = summarizeRun(run([suite(1, 0, 'passed'), suite(2, 0, 'passed')], 3))
    expect(s.verdict).toBe('awaiting')
    expect(s.glyph).toBe('○')
    expect(s.awaiting).toBe(3)
  })

  it('gives a failing run no verdict either, but still counts what failed', () => {
    const s = summarizeRun(run([suite(1, 4, 'degraded'), suite(2, 0, 'passed')], 1))
    expect(s.verdict).toBe('awaiting')
    expect(s.glyph).toBe('○')
    expect(s.failedCount).toBe(4)
    expect(s.suitesFailing).toBe(1)
  })

  it.each([undefined, 0])('keeps the verdict when awaiting_suites is %s', (awaiting) => {
    expect(summarizeRun(run([suite(1, 0, 'passed')], awaiting))).toMatchObject({
      verdict: 'passed',
      glyph: '✓',
      awaiting: 0,
    })
    expect(summarizeRun(run([suite(1, 2, 'degraded')], awaiting))).toMatchObject({
      verdict: 'failed',
      glyph: '✗',
      awaiting: 0,
    })
  })
})
