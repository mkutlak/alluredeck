import { render, screen } from '@testing-library/react'
import { FailureBadges } from '../FailureBadges'

// A title reaches neither touch nor screen readers: new, inherited and known also spell
// out their meaning in screen-reader text, as env already does. Flaky has a title only
// (FlakyBadge is shared).
describe('FailureBadges for screen readers', () => {
  it.each([
    { flags: { newFailed: true }, text: 'new', spoken: /^new \(.*passed on the default branch/ },
    { flags: { inherited: true }, text: 'inherited', spoken: /^inherited \(.*default branch/ },
    { flags: { known: true }, text: 'known', spoken: /^known \(.*known issue/i },
  ])('spells out "$text"', ({ flags, text, spoken }) => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} {...flags} />)
    expect(screen.getByText(text)).toHaveTextContent(spoken)
  })

  it('explains "known" in its title', () => {
    render(<FailureBadges flaky={false} newFailed={false} known />)
    expect(screen.getByText('known')).toHaveAttribute(
      'title',
      expect.stringMatching(/known issue/i),
    )
  })

  it('explains "flaky" in its title', () => {
    render(<FailureBadges flaky newFailed={false} known={false} retries={2} />)
    expect(screen.getByTestId('flaky-badge')).toHaveAttribute(
      'title',
      expect.stringMatching(/^Flaky: .*retry/),
    )
  })
})
