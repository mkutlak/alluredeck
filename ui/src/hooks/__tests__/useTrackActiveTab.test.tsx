import { describe, it, expect, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { createElement, type ReactNode } from 'react'
import { useUIStore } from '@/store/ui'
import { projectNavItems } from '@/lib/projectNav'
import { useTrackActiveTab } from '../useTrackActiveTab'

beforeEach(() => {
  useUIStore.setState({ lastTabPerProject: {} })
})

function renderAt(path: string, projectId: string | null) {
  renderHook(() => useTrackActiveTab(projectId), {
    wrapper: ({ children }: { children: ReactNode }) =>
      createElement(MemoryRouter, { initialEntries: [path] }, children),
  })
  return useUIStore.getState().lastTabPerProject
}

describe('useTrackActiveTab', () => {
  // Every tab the project nav offers is remembered ('' is Overview).
  const navPaths = projectNavItems({ project_id: 42, slug: 'p' }).map((item) => item.to)

  it.each(navPaths)('records the tab for %s', (path) => {
    expect(renderAt(path, '42')).toEqual({ '42': path.replace(/^\/projects\/42\/?/, '') })
  })

  it.each([
    ['a deep sub-route', '/projects/42/reports/123', '42'],
    ['a non-tab segment', '/projects/42/compare', '42'],
    ['the tests page', '/projects/42/tests', '42'],
    ['a null projectId', '/projects/42/analytics', null],
    ['a non-project route', '/', null],
  ])('is a no-op for %s', (_, path, projectId) => {
    expect(renderAt(path, projectId)).toEqual({})
  })

  it('does not overwrite a stored tab when on a deep route', () => {
    useUIStore.setState({ lastTabPerProject: { '42': 'analytics' } })
    expect(renderAt('/projects/42/reports/123', '42')).toEqual({ '42': 'analytics' })
  })
})
