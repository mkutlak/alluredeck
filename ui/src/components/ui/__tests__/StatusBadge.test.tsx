import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StatusBadge } from '../StatusBadge'
import { STATUS_BADGE_CLASSES } from '@/lib/status-colors'

describe('StatusBadge', () => {
  it.each(['passed', 'failed', 'broken', 'skipped'] as const)(
    'renders %s status with its variant classes',
    (status) => {
      render(<StatusBadge status={status} />)
      const el = screen.getByText(status)
      for (const cls of STATUS_BADGE_CLASSES[status].split(' ')) {
        expect(el.className).toContain(cls)
      }
    },
  )

  it('renders unknown status with the secondary variant', () => {
    render(<StatusBadge status="unknown" />)
    const el = screen.getByText('unknown')
    expect(el.className).toContain('bg-secondary')
  })

  it('renders an unrecognized string with the secondary variant', () => {
    render(<StatusBadge status="weird-status" />)
    const el = screen.getByText('weird-status')
    expect(el.className).toContain('bg-secondary')
  })

  it('keeps the label lowercase as-is', () => {
    render(<StatusBadge status="FAILED" />)
    expect(screen.getByText('FAILED')).toBeInTheDocument()
  })

  it('applies className passthrough', () => {
    render(<StatusBadge status="passed" className="custom-class" />)
    expect(screen.getByText('passed').className).toContain('custom-class')
  })
})
