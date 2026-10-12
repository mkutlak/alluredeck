import { describe, it, expect } from 'vitest'

import { countOrigins, failureBucket, originParts } from '../failureOrigin'

describe('failureBucket', () => {
  it.each([
    { row: { origin: 'new', environment: false }, want: 'new' },
    { row: { origin: 'inherited', environment: false }, want: 'inherited' },
    { row: { origin: 'unknown', environment: false }, want: 'unknown' },
    // Owner decision 2026-10-07: an environment failure is never "new".
    { row: { origin: 'new', environment: true }, want: 'inherited' },
    { row: { origin: 'unknown', environment: true }, want: 'inherited' },
    { row: {}, want: 'unknown' },
  ] as const)('puts $row in $want', ({ row, want }) => {
    expect(failureBucket(row)).toBe(want)
  })
})

describe('countOrigins', () => {
  it('counts each bucket once, with env as a subset of inherited', () => {
    expect(
      countOrigins([
        { origin: 'new' },
        { origin: 'new' },
        { origin: 'new', environment: true },
        { origin: 'inherited' },
        { origin: 'unknown' },
        {},
      ]),
    ).toEqual({ new: 2, inherited: 2, unknown: 2, env: 1 })
  })
})

describe('originParts', () => {
  it.each([
    {
      counts: { new: 3, inherited: 2, unknown: 1, env: 1 },
      want: ['3 new', '2 inherited (1 env)', '1 unknown'],
    },
    { counts: { new: 0, inherited: 4, unknown: 0, env: 0 }, want: ['none new', '4 inherited'] },
    { counts: { new: 1, inherited: 0, unknown: 0, env: 0 }, want: ['1 new'] },
    { counts: { new: 0, inherited: 0, unknown: 0, env: 0 }, want: [] },
  ])('reads $counts as $want', ({ counts, want }) => {
    expect(originParts(counts)).toEqual(want)
  })

  // A truncated response only lists some failures: the counts say whose they are, once.
  it('scopes the counts to the listed failures when the response was truncated', () => {
    expect(originParts({ new: 300, inherited: 150, unknown: 50, env: 10 }, 500)).toEqual([
      'first 500: 300 new',
      '150 inherited (10 env)',
      '50 unknown',
    ])
  })
})

describe('unexpected origin values', () => {
  // '' or a value from a newer API must still land in a bucket, never out of the counts.
  it.each(['', 'baseline-missing'])('reads origin %j as unknown', (origin) => {
    const row = { origin } as unknown as Parameters<typeof failureBucket>[0]
    expect(failureBucket(row)).toBe('unknown')
    expect(countOrigins([row])).toEqual({ new: 0, inherited: 0, unknown: 1, env: 0 })
  })
})
