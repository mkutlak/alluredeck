import { beforeEach, describe, it, expect } from 'vitest'
import { useUIStore } from './ui'

beforeEach(() => {
  useUIStore.setState({ projectViewMode: 'grid', lastProjectId: null })
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
