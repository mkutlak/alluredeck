import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { ProjectTabBar } from '../ProjectTabBar'

function Wrapper({ path }: { path: string }) {
  return (
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/projects/:id/*" element={<ProjectTabBar />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('ProjectTabBar', () => {
  it('renders three tabs', () => {
    render(<Wrapper path="/projects/my-project" />)
    expect(screen.getByText('Overview')).toBeInTheDocument()
    expect(screen.getByText('Analytics')).toBeInTheDocument()
    expect(screen.getByText('History')).toBeInTheDocument()
  })

  it('returns null when no projectId', () => {
    const { container } = render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route path="/" element={<ProjectTabBar />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(container.firstChild).toBeNull()
  })
})
