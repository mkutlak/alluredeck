import { describe, it, expect } from 'vitest'

import { originParts } from '../failureOrigin'

// Owner's rule: a run whose failures are all inherited answers "is it me?" in words
// before the counts. Only when that is the whole truth: nothing new, nothing unknown,
// every failure listed.
describe('originParts "none new"', () => {
  it.each([
    { counts: { new: 0, inherited: 4, unknown: 0, env: 0 }, want: ['none new', '4 inherited'] },
    {
      counts: { new: 0, inherited: 2, unknown: 0, env: 2 },
      want: ['none new', '2 inherited (2 env)'],
    },
  ])('leads with "none new" when every failure is inherited ($counts)', ({ counts, want }) => {
    expect(originParts(counts)).toEqual(want)
  })

  it.each([
    { name: 'some unknown', counts: { new: 0, inherited: 4, unknown: 1, env: 0 } },
    { name: 'some new', counts: { new: 1, inherited: 4, unknown: 0, env: 0 } },
  ])('does not say "none new" with $name', ({ counts }) => {
    expect(originParts(counts)).not.toContain('none new')
  })

  it('does not say "none new" when the failures were truncated', () => {
    expect(originParts({ new: 0, inherited: 4, unknown: 0, env: 0 }, 4)).toEqual([
      'first 4: 4 inherited',
    ])
  })
})
