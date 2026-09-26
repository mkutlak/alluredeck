import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AlertBanner } from '../AlertBanner'

describe('AlertBanner', () => {
  // info is a polite status; warning interrupts as an alert.
  it.each([
    ['info', 'status'],
    ['warning', 'alert'],
  ] as const)('renders the %s variant with role=%s', (variant, role) => {
    render(<AlertBanner variant={variant}>Heads up</AlertBanner>)
    expect(screen.getByRole(role)).toHaveTextContent('Heads up')
  })
})
