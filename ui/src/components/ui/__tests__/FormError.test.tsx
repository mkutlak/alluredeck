import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FormError } from '../FormError'

describe('FormError', () => {
  it('renders nothing when message is undefined', () => {
    const { container } = render(<FormError />)
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when message is empty string', () => {
    const { container } = render(<FormError message="" />)
    expect(container.firstChild).toBeNull()
  })

  it('renders error message with role=alert', () => {
    render(<FormError message="Something went wrong" />)
    const el = screen.getByRole('alert')
    expect(el).toBeInTheDocument()
    expect(el).toHaveTextContent('Something went wrong')
  })

  it('applies destructive text styling', () => {
    render(<FormError message="Error" />)
    const el = screen.getByRole('alert')
    expect(el.className).toContain('destructive')
  })
})
