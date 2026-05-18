import { describe, it, expect, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { createElement } from 'react'
import { useUIStore } from '@/store/ui'
import { useTrackActiveTab } from '../useTrackActiveTab'

// Reset store between tests
beforeEach(() => {
  useUIStore.setState({ lastTabPerProject: {} })
})

function TestHook({ projectId }: { projectId: string | null }) {
  useTrackActiveTab(projectId)
  return null
}

function renderAt(path: string, projectId: string | null) {
  render(
    createElement(
      MemoryRouter,
      { initialEntries: [path] },
      createElement(
        Routes,
        null,
        // Match both the exact project route and sub-routes
        createElement(Route, {
          path: '/projects/:id/*',
          element: createElement(TestHook, { projectId }),
        }),
        createElement(Route, {
          path: '/*',
          element: createElement(TestHook, { projectId }),
        }),
      ),
    ),
  )
}

describe('useTrackActiveTab', () => {
  it('records the Overview tab (empty string) for bare /projects/:id', () => {
    renderAt('/projects/42', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('')
  })

  it('records the analytics tab', () => {
    renderAt('/projects/42/analytics', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('analytics')
  })

  it('records the defects tab', () => {
    renderAt('/projects/42/defects', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('defects')
  })

  it('records the timeline tab', () => {
    renderAt('/projects/42/timeline', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('timeline')
  })

  it('records the known-issues tab', () => {
    renderAt('/projects/42/known-issues', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('known-issues')
  })

  it('records the attachments tab', () => {
    renderAt('/projects/42/attachments', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('attachments')
  })

  it('is a no-op for deep sub-route reports/...', () => {
    renderAt('/projects/42/reports/123', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBeUndefined()
  })

  it('is a no-op for deep sub-route trace/...', () => {
    renderAt('/projects/42/trace/abc', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBeUndefined()
  })

  it('is a no-op for compare sub-route', () => {
    renderAt('/projects/42/compare', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBeUndefined()
  })

  it('is a no-op for tests sub-route', () => {
    renderAt('/projects/42/tests', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBeUndefined()
  })

  it('is a no-op when projectId is null', () => {
    renderAt('/projects/42/analytics', null)
    expect(useUIStore.getState().lastTabPerProject['42']).toBeUndefined()
  })

  it('is a no-op when not on a project route', () => {
    renderAt('/', null)
    expect(Object.keys(useUIStore.getState().lastTabPerProject)).toHaveLength(0)
  })

  it('does not overwrite a stored tab when on a deep route', () => {
    useUIStore.setState({ lastTabPerProject: { '42': 'analytics' } })
    renderAt('/projects/42/reports/123', '42')
    expect(useUIStore.getState().lastTabPerProject['42']).toBe('analytics')
  })
})
