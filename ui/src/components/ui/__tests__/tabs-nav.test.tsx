import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { TabsNav } from '../tabs-nav'

const items = [
  { to: '/projects/1', label: 'Overview', end: true },
  { to: '/projects/1/analytics', label: 'Analytics' },
]

describe('TabsNav', () => {
  // `end` keeps the index tab from matching every sub-route.
  it('marks the analytics route active when on that route', () => {
    render(
      <MemoryRouter initialEntries={['/projects/1/analytics']}>
        <TabsNav items={items} aria-label="Project sections" />
      </MemoryRouter>,
    )
    expect(screen.getByRole('link', { name: 'Analytics' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Overview' })).not.toHaveAttribute('aria-current')
  })
})
