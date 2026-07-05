import { render, screen } from '@testing-library/react'
import { FlakyBadge } from '../FlakyBadge'

describe('FlakyBadge', () => {
  it('renders the base "flaky" label when no retries are given', () => {
    render(<FlakyBadge />)
    expect(screen.getByText('flaky')).toBeInTheDocument()
  })

  it('renders the base label when retries is 0', () => {
    render(<FlakyBadge retries={0} />)
    expect(screen.getByText('flaky')).toBeInTheDocument()
  })

  it('appends the retry count when retries is greater than 0', () => {
    render(<FlakyBadge retries={3} />)
    expect(screen.getByText('flaky · 3x')).toBeInTheDocument()
  })

  it('exposes a stable test id for reuse across surfaces', () => {
    render(<FlakyBadge />)
    expect(screen.getByTestId('flaky-badge')).toBeInTheDocument()
  })

  it('merges a custom className with the default styling', () => {
    render(<FlakyBadge className="custom-class" />)
    expect(screen.getByTestId('flaky-badge')).toHaveClass('custom-class')
  })
})
