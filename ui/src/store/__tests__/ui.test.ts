import { beforeEach, describe, it, expect } from 'vitest'
import { useUIStore } from '../ui'

const store = () => useUIStore.getState()

beforeEach(() => {
  useUIStore.setState({
    pinnedProjectIds: [],
    recentProjectIds: [],
    lastTabPerProject: {},
    runsFeedGroupIds: [],
  })
})

describe('useUIStore', () => {
  it('pinProject appends each id once; unpinProject removes it and ignores unknown ids', () => {
    for (const id of [1, 2, 1, 3]) store().pinProject(id)
    expect(store().pinnedProjectIds).toEqual([1, 2, 3])

    store().unpinProject(99)
    store().unpinProject(1)
    expect(store().pinnedProjectIds).toEqual([2, 3])
  })

  it('recordProjectVisit keeps the 5 most recent distinct ids, newest first', () => {
    for (const id of [1, 2, 3, 1]) store().recordProjectVisit(id)
    expect(store().recentProjectIds).toEqual([1, 3, 2])

    for (const id of [4, 5, 6]) store().recordProjectVisit(id)
    expect(store().recentProjectIds).toEqual([6, 5, 4, 1, 3])
  })

  it('setLastTabForProject stores one tab per project, overwriting that project only', () => {
    store().setLastTabForProject('1', 'analytics')
    store().setLastTabForProject('2', '')
    store().setLastTabForProject('1', 'defects')
    expect(store().lastTabPerProject).toEqual({ '1': 'defects', '2': '' })
  })

  it('setRunsFeedGroupIds stores a new array reference (immutable)', () => {
    const input = [1, 2]
    store().setRunsFeedGroupIds(input)
    expect(store().runsFeedGroupIds).not.toBe(input)
    expect(store().runsFeedGroupIds).toEqual(input)
  })
})
