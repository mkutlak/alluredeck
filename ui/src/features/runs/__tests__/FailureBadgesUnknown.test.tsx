import { render, screen } from '@testing-library/react'
import { FailureBadges } from '../FailureBadges'

// A failure with no pass or fail on the default branch to compare with is tagged, not
// left bare, so "no tag" never reads as "fine".
describe('FailureBadges unknown origin', () => {
  it('tags it "unknown" with a title and screen-reader text', () => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} unknown />)
    const badge = screen.getByText('unknown')
    expect(badge).toHaveAttribute(
      'title',
      'Unknown: the default branch has no pass or fail of this test to compare with',
    )
    expect(badge).toHaveTextContent(
      /^unknown \(no pass or fail on the default branch to compare with\)$/,
    )
  })

  it('puts the explanation in screen-reader text, not on screen', () => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} unknown />)
    expect(screen.getByText(/no pass or fail on the default branch/)).toHaveClass('sr-only')
  })

  // Any origin that says more wins: unknown is only the answer when nothing else is known.
  it.each([
    { name: 'environment', flags: { environment: true } },
    { name: 'new', flags: { newFailed: true } },
    { name: 'inherited', flags: { inherited: true } },
  ])('is not shown beside $name', ({ flags }) => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} unknown {...flags} />)
    expect(screen.queryByText('unknown')).not.toBeInTheDocument()
  })

  it('sits beside the known issue badge without running into it', () => {
    render(<FailureBadges flaky={false} newFailed={false} known unknown />)
    expect(screen.getByText('unknown')).toBeInTheDocument()
    expect(screen.getByText('known issue')).toBeInTheDocument()
  })
})
