import { render, screen } from '@testing-library/react'
import { FlakyBadge } from '../FlakyBadge'

describe('FlakyBadge', () => {
  it.each([
    [undefined, 'flaky'],
    [0, 'flaky'],
    [3, 'flaky · 3x'],
  ])('retries=%s renders %j', (retries, label) => {
    render(<FlakyBadge retries={retries} />)
    expect(screen.getByTestId('flaky-badge').textContent).toBe(label)
  })
})
