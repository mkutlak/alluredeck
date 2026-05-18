import { beforeEach, describe, it, expect } from 'vitest'
import { useUIStore } from '../ui'

beforeEach(() => {
  useUIStore.setState({
    pinnedProjectIds: [],
    recentProjectIds: [],
    lastTabPerProject: {},
  })
})

describe('useUIStore - pinProject / unpinProject', () => {
  it('pinProject adds an id', () => {
    useUIStore.getState().pinProject(1)
    expect(useUIStore.getState().pinnedProjectIds).toEqual([1])
  })

  it('pinProject is idempotent — adding the same id twice keeps only one entry', () => {
    useUIStore.getState().pinProject(1)
    useUIStore.getState().pinProject(1)
    expect(useUIStore.getState().pinnedProjectIds).toEqual([1])
  })

  it('pinProject can add multiple distinct ids', () => {
    useUIStore.getState().pinProject(1)
    useUIStore.getState().pinProject(2)
    useUIStore.getState().pinProject(3)
    expect(useUIStore.getState().pinnedProjectIds).toEqual([1, 2, 3])
  })

  it('unpinProject removes an existing id', () => {
    useUIStore.getState().pinProject(1)
    useUIStore.getState().pinProject(2)
    useUIStore.getState().unpinProject(1)
    expect(useUIStore.getState().pinnedProjectIds).toEqual([2])
  })

  it('unpinProject is a no-op when id is not pinned', () => {
    useUIStore.getState().pinProject(1)
    useUIStore.getState().unpinProject(99)
    expect(useUIStore.getState().pinnedProjectIds).toEqual([1])
  })

  it('unpinProject on empty list is a no-op', () => {
    useUIStore.getState().unpinProject(1)
    expect(useUIStore.getState().pinnedProjectIds).toEqual([])
  })
})

describe('useUIStore - recordProjectVisit', () => {
  it('prepends the visited id (most-recently-visited first)', () => {
    useUIStore.getState().recordProjectVisit(1)
    useUIStore.getState().recordProjectVisit(2)
    expect(useUIStore.getState().recentProjectIds).toEqual([2, 1])
  })

  it('deduplicates: revisiting an existing id moves it to front', () => {
    useUIStore.getState().recordProjectVisit(1)
    useUIStore.getState().recordProjectVisit(2)
    useUIStore.getState().recordProjectVisit(3)
    useUIStore.getState().recordProjectVisit(1)
    expect(useUIStore.getState().recentProjectIds).toEqual([1, 3, 2])
  })

  it('trims the list to a maximum of 5 entries', () => {
    for (const id of [1, 2, 3, 4, 5, 6]) {
      useUIStore.getState().recordProjectVisit(id)
    }
    const ids = useUIStore.getState().recentProjectIds
    expect(ids).toHaveLength(5)
    expect(ids[0]).toBe(6)
  })

  it('trim keeps the 5 most-recently-visited, discarding the oldest', () => {
    for (const id of [1, 2, 3, 4, 5, 6]) {
      useUIStore.getState().recordProjectVisit(id)
    }
    expect(useUIStore.getState().recentProjectIds).toEqual([6, 5, 4, 3, 2])
  })

  it('visiting the same id repeatedly stays at length 1', () => {
    useUIStore.getState().recordProjectVisit(7)
    useUIStore.getState().recordProjectVisit(7)
    useUIStore.getState().recordProjectVisit(7)
    expect(useUIStore.getState().recentProjectIds).toEqual([7])
  })
})

describe('useUIStore - setLastTabForProject', () => {
  it('stores the tab for a given projectId string', () => {
    useUIStore.getState().setLastTabForProject('42', 'analytics')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('analytics')
  })

  it('overwrites a previous tab for the same project', () => {
    useUIStore.getState().setLastTabForProject('42', 'analytics')
    useUIStore.getState().setLastTabForProject('42', 'defects')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('defects')
  })

  it('stores tabs for multiple projects independently', () => {
    useUIStore.getState().setLastTabForProject('1', 'timeline')
    useUIStore.getState().setLastTabForProject('2', 'attachments')
    const tabs = useUIStore.getState().lastTabPerProject
    expect(tabs['1']).toBe('timeline')
    expect(tabs['2']).toBe('attachments')
  })

  it('stores empty string for the Overview tab', () => {
    useUIStore.getState().setLastTabForProject('5', '')
    expect(useUIStore.getState().lastTabPerProject['5']).toBe('')
  })
})
