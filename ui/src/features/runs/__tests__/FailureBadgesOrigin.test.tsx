import { render, screen } from '@testing-library/react'
import { FailureBadges } from '../FailureBadges'

describe('FailureBadges origin tags', () => {
  it.each([
    { flags: { inherited: true }, shown: 'inherited' },
    { flags: { environment: true }, shown: 'env' },
  ])('shows "$shown" for $flags', ({ flags, shown }) => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} {...flags} />)
    expect(screen.getByText(shown, { exact: true })).toBeInTheDocument()
  })

  // An environment failure is labelled env only: never also inherited or new.
  it('shows env alone even when the row is also marked inherited or new', () => {
    render(<FailureBadges flaky={false} newFailed known={false} inherited environment />)
    expect(screen.getByText('env')).toBeInTheDocument()
    expect(screen.queryByText('inherited')).not.toBeInTheDocument()
    expect(screen.queryByText('new')).not.toBeInTheDocument()
  })

  // Words carry the meaning; a tooltip says what each origin means.
  it.each([
    { flags: { newFailed: true }, text: 'new', title: /passed on the default branch/i },
    { flags: { inherited: true }, text: 'inherited', title: /failing on the default branch/i },
    { flags: { environment: true }, text: 'env', title: /environment/i },
  ])('explains "$text" in its title', ({ flags, text, title }) => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} {...flags} />)
    expect(screen.getByText(text)).toHaveAttribute('title', expect.stringMatching(title))
  })
})

describe('FailureBadges as labels', () => {
  // Badge's default variant fills solid blue on hover; a label must not act like a control.
  it('never fills any tag on hover', () => {
    render(<FailureBadges flaky newFailed known retries={2} />)
    render(<FailureBadges flaky={false} newFailed={false} known={false} inherited />)
    render(<FailureBadges flaky={false} newFailed={false} known={false} environment />)
    for (const text of [/^flaky/, 'new', 'known', 'inherited', 'env']) {
      expect(screen.getByText(text).className).not.toMatch(/hover:/)
    }
  })

  it('spells out env for screen readers, not only in a tooltip', () => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} environment />)
    expect(screen.getByText('env')).toHaveTextContent('env (environment failure)')
    expect(screen.getByText(/environment failure/)).toHaveClass('sr-only')
  })
})
