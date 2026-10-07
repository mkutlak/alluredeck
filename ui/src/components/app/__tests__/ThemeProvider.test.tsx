import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ThemeProvider } from '../ThemeProvider'

// React warns once per module load ("Encountered a script tag while rendering React
// component"), so this must stay the first and only render in this file.
describe('ThemeProvider', () => {
  it('renders children without React warning about the no-flash <script>', () => {
    // jsdom has no matchMedia; next-themes reads it for the "system" theme.
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addListener: vi.fn(),
      removeListener: vi.fn(),
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ThemeProvider>
        <p>child</p>
      </ThemeProvider>,
    )

    expect(screen.getByText('child')).toBeInTheDocument()
    const messages = consoleError.mock.calls.map((args) => args.map(String).join(' '))
    expect(messages.filter((m) => /script tag/i.test(m))).toEqual([])

    consoleError.mockRestore()
  })
})
