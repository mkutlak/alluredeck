import { render, screen } from '@testing-library/react'
import { FailureBadges } from '../FailureBadges'

describe('FailureBadges', () => {
  it('renders no badges when all flags are false', () => {
    render(<FailureBadges flaky={false} newFailed={false} known={false} />)
    expect(screen.queryByText('flaky')).not.toBeInTheDocument()
    expect(screen.queryByText('new')).not.toBeInTheDocument()
    expect(screen.queryByText('known')).not.toBeInTheDocument()
  })

  it('renders the flaky badge when flaky is true', () => {
    render(<FailureBadges flaky={true} newFailed={false} known={false} />)
    expect(screen.getByText('flaky')).toBeInTheDocument()
  })

  it('renders the new badge when newFailed is true', () => {
    render(<FailureBadges flaky={false} newFailed={true} known={false} />)
    expect(screen.getByText('new')).toBeInTheDocument()
  })

  it('renders the known badge when known is true', () => {
    render(<FailureBadges flaky={false} newFailed={false} known={true} />)
    expect(screen.getByText('known')).toBeInTheDocument()
  })

  it('renders all three badges together', () => {
    render(<FailureBadges flaky={true} newFailed={true} known={true} />)
    expect(screen.getByText('flaky')).toBeInTheDocument()
    expect(screen.getByText('new')).toBeInTheDocument()
    expect(screen.getByText('known')).toBeInTheDocument()
  })
})
