import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AlertBanner } from '../AlertBanner'

describe('AlertBanner', () => {
  it('renders info variant with role=status and info classes', () => {
    render(<AlertBanner variant="info">Heads up</AlertBanner>)
    const el = screen.getByRole('status')
    expect(el).toHaveTextContent('Heads up')
    expect(el.className).toContain('border-info/30')
    expect(el.className).toContain('bg-info/10')
    expect(el.className).toContain('text-info')
  })

  it('renders warning variant with role=alert and warning classes', () => {
    render(<AlertBanner variant="warning">Careful</AlertBanner>)
    const el = screen.getByRole('alert')
    expect(el).toHaveTextContent('Careful')
    expect(el.className).toContain('border-warning/30')
    expect(el.className).toContain('bg-warning/10')
    expect(el.className).toContain('text-warning')
  })

  it('merges className overrides', () => {
    render(
      <AlertBanner variant="info" className="custom-class">
        Heads up
      </AlertBanner>,
    )
    expect(screen.getByRole('status').className).toContain('custom-class')
  })
})
