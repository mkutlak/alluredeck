import { render, screen } from '@testing-library/react'
import { FailureBadges } from '../FailureBadges'

// The 10px new/flaky/known badges sit on a 15% tint of their hue. Their light-theme hue text
// was 2.3-3.8:1 on that tint (flaky peach worst), so in light mode the text is ink instead.
// The hue stays in the tint and, because dark-mode hues are legible, in dark-mode text.
describe('FailureBadges ink', () => {
  it.each([
    {
      text: /^flaky/,
      tint: 'bg-[#fe640b]/15',
      dark: 'dark:text-[#fab387]',
    },
    {
      text: 'new',
      tint: 'bg-[#1e66f5]/15',
      dark: 'dark:text-[#89b4fa]',
    },
    {
      text: 'known issue',
      tint: 'bg-[#8c8fa1]/15',
      dark: 'dark:text-[#a6adc8]',
    },
  ])('$text is ink in light mode, keeping its tint and dark hue', ({ text, tint, dark }) => {
    render(<FailureBadges flaky newFailed known retries={2} />)
    const badge = screen.getByText(text)
    expect(badge).toHaveClass('text-foreground')
    expect(badge.className).not.toMatch(/(^|\s)text-\[#/)
    expect(badge).toHaveClass(tint)
    expect(badge).toHaveClass(dark)
  })
})
