import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { Route, Routes, useLocation } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { ProjectGuard } from '../ProjectGuard'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 7, slug: 'my-slug' }],
    metadata: { message: 'ok' },
  }),
}))

function Location() {
  const { pathname, search, hash } = useLocation()
  return <div data-testid="location">{pathname + search + hash}</div>
}

function renderGuard(path: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/projects/:id" element={<ProjectGuard />}>
        <Route path="attachments" element={<Location />} />
      </Route>
    </Routes>,
    { route: path },
  )
}

describe('ProjectGuard', () => {
  it('redirects a slug URL to the numeric project id, keeping the tab, query and hash', async () => {
    renderGuard('/projects/my-slug/attachments?status=failed#top')

    expect(await screen.findByTestId('location')).toHaveTextContent(
      '/projects/7/attachments?status=failed#top',
    )
  })

  it('leaves a numeric URL as it is', async () => {
    renderGuard('/projects/7/attachments')

    expect(await screen.findByTestId('location')).toHaveTextContent('/projects/7/attachments')
  })
})
