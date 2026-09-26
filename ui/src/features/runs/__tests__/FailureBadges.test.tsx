import { render, screen } from '@testing-library/react'
import { FailureBadges } from '../FailureBadges'

const BADGES = { flaky: /^flaky/, new: /^new$/, known: /^known$/ }

describe('FailureBadges', () => {
  it.each([
    { flags: { flaky: false, newFailed: false, known: false }, want: [] },
    { flags: { flaky: true, newFailed: false, known: false }, want: ['flaky'] },
    { flags: { flaky: false, newFailed: true, known: false }, want: ['new'] },
    { flags: { flaky: false, newFailed: false, known: true }, want: ['known'] },
  ])('shows exactly the $want badges', ({ flags, want }) => {
    render(<FailureBadges {...flags} />)
    for (const [name, text] of Object.entries(BADGES)) {
      expect(screen.queryByText(text) !== null).toBe(want.includes(name))
    }
  })

  it('shows the retry count on the flaky badge when retries is given', () => {
    render(<FailureBadges flaky={true} newFailed={false} known={false} retries={3} />)
    expect(screen.getByText('flaky · 3x')).toBeInTheDocument()
  })
})
