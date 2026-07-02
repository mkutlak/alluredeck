import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PageHeader } from '../PageHeader'

describe('PageHeader', () => {
  it('renders the title', () => {
    render(<PageHeader title="Projects" />)
    expect(screen.getByRole('heading', { name: 'Projects' })).toBeInTheDocument()
  })

  it('renders the subtitle when provided', () => {
    render(<PageHeader title="Projects" subtitle="12 projects" />)
    expect(screen.getByText('12 projects')).toBeInTheDocument()
  })

  it('renders the meta row when provided', () => {
    render(<PageHeader title="Projects" meta={<span data-testid="meta-row">chips</span>} />)
    expect(screen.getByTestId('meta-row')).toBeInTheDocument()
  })

  it('renders actions when provided', () => {
    render(<PageHeader title="Projects" actions={<button>Refresh</button>} />)
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
  })

  it('renders the toolbar row when provided', () => {
    render(<PageHeader title="Projects" toolbar={<div data-testid="toolbar-row">toolbar</div>} />)
    expect(screen.getByTestId('toolbar-row')).toBeInTheDocument()
  })

  it('defaults titleVariant to mono (font-mono applied)', () => {
    render(<PageHeader title="proj-alpha" />)
    expect(screen.getByRole('heading', { name: 'proj-alpha' }).className).toContain('font-mono')
  })

  it('omits font-mono when titleVariant is sans', () => {
    render(<PageHeader title="Projects" titleVariant="sans" />)
    expect(screen.getByRole('heading', { name: 'Projects' }).className).not.toContain('font-mono')
  })

  it('does not render an empty subtitle paragraph when subtitle is absent', () => {
    const { container } = render(<PageHeader title="Projects" />)
    expect(container.querySelector('p')).not.toBeInTheDocument()
  })
})
