import { beforeEach, describe, it, expect } from 'vitest'
import { useUIStore } from './ui'

beforeEach(() => {
  useUIStore.setState({
    projectViewMode: 'grid',
    lastProjectId: null,
    reportsPerPage: 20,
    reportsGroupBy: 'none',
  })
})

describe('useUIStore - projectViewMode', () => {
  it('defaults to grid', () => {
    expect(useUIStore.getState().projectViewMode).toBe('grid')
  })

  it('setProjectViewMode updates the view mode', () => {
    useUIStore.getState().setProjectViewMode('table')
    expect(useUIStore.getState().projectViewMode).toBe('table')
  })
})

describe('useUIStore - reportsPerPage', () => {
  it('defaults to 20', () => {
    expect(useUIStore.getState().reportsPerPage).toBe(20)
  })

  it('setReportsPerPage updates the value', () => {
    useUIStore.getState().setReportsPerPage(50)
    expect(useUIStore.getState().reportsPerPage).toBe(50)
  })

  it('setReportsPerPage accepts all valid options', () => {
    for (const n of [10, 20, 50, 100]) {
      useUIStore.getState().setReportsPerPage(n)
      expect(useUIStore.getState().reportsPerPage).toBe(n)
    }
  })
})

describe('useUIStore - reportsGroupBy', () => {
  it("defaults to 'none'", () => {
    expect(useUIStore.getState().reportsGroupBy).toBe('none')
  })

  it('setReportsGroupBy updates to commit', () => {
    useUIStore.getState().setReportsGroupBy('commit')
    expect(useUIStore.getState().reportsGroupBy).toBe('commit')
  })

  it('setReportsGroupBy updates to branch', () => {
    useUIStore.getState().setReportsGroupBy('branch')
    expect(useUIStore.getState().reportsGroupBy).toBe('branch')
  })

  it('setReportsGroupBy updates back to none', () => {
    useUIStore.getState().setReportsGroupBy('commit')
    useUIStore.getState().setReportsGroupBy('none')
    expect(useUIStore.getState().reportsGroupBy).toBe('none')
  })
})

describe('useUIStore - lastProjectId', () => {
  it('defaults to null', () => {
    expect(useUIStore.getState().lastProjectId).toBeNull()
  })

  it('setLastProjectId stores a string id', () => {
    useUIStore.getState().setLastProjectId('foo')
    expect(useUIStore.getState().lastProjectId).toBe('foo')
  })

  it('setLastProjectId stores null', () => {
    useUIStore.getState().setLastProjectId('foo')
    useUIStore.getState().setLastProjectId(null)
    expect(useUIStore.getState().lastProjectId).toBeNull()
  })

  it('clearLastProjectId resets to null', () => {
    useUIStore.getState().setLastProjectId('foo')
    useUIStore.getState().clearLastProjectId()
    expect(useUIStore.getState().lastProjectId).toBeNull()
  })
})
