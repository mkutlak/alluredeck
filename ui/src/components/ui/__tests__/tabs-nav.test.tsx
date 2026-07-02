import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { TabsNav } from '../tabs-nav'

const items = [
  { to: '/projects/1', label: 'Overview', end: true, 'data-testid': 'sidebar-nav-overview' },
  { to: '/projects/1/analytics', label: 'Analytics', 'data-testid': 'sidebar-nav-analytics' },
]

function renderNav(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <TabsNav items={items} aria-label="Project sections" />
    </MemoryRouter>,
  )
}

describe('TabsNav', () => {
  it('renders a nav landmark with the given aria-label', () => {
    renderNav('/projects/1')
    expect(screen.getByRole('navigation', { name: 'Project sections' })).toBeInTheDocument()
  })

  it('renders a link for each item with its label', () => {
    renderNav('/projects/1')
    expect(screen.getByRole('link', { name: 'Overview' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Analytics' })).toBeInTheDocument()
  })

  it('links point to the given "to" paths', () => {
    renderNav('/projects/1')
    expect(screen.getByRole('link', { name: 'Overview' })).toHaveAttribute('href', '/projects/1')
    expect(screen.getByRole('link', { name: 'Analytics' })).toHaveAttribute(
      'href',
      '/projects/1/analytics',
    )
  })

  it('passes through data-testid on each link', () => {
    renderNav('/projects/1')
    expect(screen.getByTestId('sidebar-nav-overview')).toBeInTheDocument()
    expect(screen.getByTestId('sidebar-nav-analytics')).toBeInTheDocument()
  })

  it('marks the active route with aria-current="page"', () => {
    renderNav('/projects/1')
    expect(screen.getByRole('link', { name: 'Overview' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Analytics' })).not.toHaveAttribute('aria-current')
  })

  it('applies active styling classes to the active tab', () => {
    renderNav('/projects/1')
    const active = screen.getByRole('link', { name: 'Overview' })
    expect(active.className).toContain('border-primary')
    expect(active.className).toContain('text-foreground')
  })

  it('applies inactive styling classes to non-active tabs', () => {
    renderNav('/projects/1')
    const inactive = screen.getByRole('link', { name: 'Analytics' })
    expect(inactive.className).not.toContain('border-primary')
  })

  it('marks the analytics route active when on that route', () => {
    renderNav('/projects/1/analytics')
    expect(screen.getByRole('link', { name: 'Analytics' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Overview' })).not.toHaveAttribute('aria-current')
  })
})
