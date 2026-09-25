import { describe, it, expect } from 'vitest'

import {
  deriveHeaderChips,
  resolveEffectiveBranch,
  splitReportHistory,
  toggleCompareSelection,
} from '../overviewHelpers'
import type { AllureStatistic, ReportHistoryEntry } from '@/types/api'

function stats(passed: number, total: number, skipped = 0): AllureStatistic {
  return { passed, failed: total - passed - skipped, broken: 0, skipped, unknown: 0, total }
}

function report(id: string, overrides: Partial<ReportHistoryEntry> = {}): ReportHistoryEntry {
  return {
    report_id: id,
    is_latest: false,
    generated_at: '2024-01-15T10:00:00Z',
    duration_ms: 5000,
    statistic: stats(10, 10),
    ...overrides,
  }
}

describe('resolveEffectiveBranch', () => {
  const branches = [{ name: 'main' }, { name: 'develop' }]

  // The selected branch is shared across projects, so a stored branch this
  // project does not have must not filter its history.
  it.each([
    {
      name: 'keeps a stored branch the project has',
      selected: 'main',
      list: branches,
      want: 'main',
    },
    {
      name: 'drops a stored branch missing from the project list',
      selected: 'missing-branch',
      list: branches,
      want: undefined,
    },
    {
      name: 'is unfiltered with no stored branch',
      selected: undefined,
      list: branches,
      want: undefined,
    },
    {
      name: 'is unfiltered until branches load',
      selected: 'main',
      list: undefined,
      want: undefined,
    },
  ])('$name', ({ selected, list, want }) => {
    expect(resolveEffectiveBranch(selected, list)).toBe(want)
  })
})

describe('splitReportHistory', () => {
  it('drops the synthetic "latest" alias from the table and exposes it as latest', () => {
    const alias = report('latest', { is_latest: true })
    const { latest, tableReports } = splitReportHistory([alias, report('41'), report('40')])

    expect(latest).toBe(alias)
    expect(tableReports.map((r) => r.report_id)).toEqual(['41', '40'])
  })

  it('leaves the table empty when the page holds only the alias', () => {
    expect(splitReportHistory([report('latest', { is_latest: true })]).tableReports).toEqual([])
  })

  it('keeps a numbered build flagged is_latest in the table', () => {
    const newest = report('42', { is_latest: true })
    const { latest, tableReports } = splitReportHistory([newest, report('41')])

    expect(latest).toBe(newest)
    expect(tableReports.map((r) => r.report_id)).toEqual(['42', '41'])
  })
})

describe('deriveHeaderChips', () => {
  // The API prepends the "latest" alias unfiltered by branch, so the chips must
  // skip it and use the newest branch-filtered numbered build.
  it('uses the newest numbered build, not the unfiltered "latest" alias', () => {
    const numbered = report('42', { statistic: stats(10, 10) })
    const { chipLatest, passRate } = deriveHeaderChips([
      report('latest', { is_latest: true, statistic: stats(1, 10) }),
      numbered,
    ])

    expect(chipLatest).toBe(numbered)
    expect(passRate).toBe(100)
  })

  it('excludes skipped tests from the pass rate', () => {
    expect(deriveHeaderChips([report('7', { statistic: stats(6, 10, 2) })]).passRate).toBe(75)
  })

  it.each([
    { name: 'there is no numbered build', reports: [report('latest', { is_latest: true })] },
    { name: 'the build has no statistic', reports: [report('7', { statistic: null })] },
  ])('has no pass rate when $name', ({ reports }) => {
    expect(deriveHeaderChips(reports).passRate).toBeNull()
  })
})

describe('toggleCompareSelection', () => {
  it.each([
    { name: 'selects into an empty selection', selected: [], id: 'a', want: ['a'] },
    { name: 'selects a second build', selected: ['a'], id: 'b', want: ['a', 'b'] },
    { name: 'ignores a third build', selected: ['a', 'b'], id: 'c', want: ['a', 'b'] },
    { name: 'deselects a selected build', selected: ['a', 'b'], id: 'a', want: ['b'] },
  ])('$name', ({ selected, id, want }) => {
    const prev = new Set<string>(selected)

    expect([...toggleCompareSelection(prev, id)]).toEqual(want)
    // React state: the previous selection must never be mutated in place.
    expect([...prev]).toEqual(selected)
  })
})
