import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FormError } from '../FormError'

describe('FormError', () => {
  it('renders the message as an alert only when it is non-empty', () => {
    const { container, rerender } = render(<FormError />)
    expect(container).toBeEmptyDOMElement()

    rerender(<FormError message="" />)
    expect(container).toBeEmptyDOMElement()

    rerender(<FormError message="Something went wrong" />)
    expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong')
  })
})
