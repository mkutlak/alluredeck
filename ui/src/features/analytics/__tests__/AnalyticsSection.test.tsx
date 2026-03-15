import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AnalyticsSection } from '../AnalyticsSection'

describe('AnalyticsSection', () => {
  it('renders title and children', () => {
    render(
      <AnalyticsSection title="Trends">
        <div data-testid="child">content</div>
      </AnalyticsSection>,
    )
    expect(screen.getByText('Trends')).toBeInTheDocument()
    expect(screen.getByTestId('child')).toBeInTheDocument()
  })

  it('returns null when isEmpty is true', () => {
    const { container } = render(
      <AnalyticsSection title="Trends" isEmpty>
        <div>content</div>
      </AnalyticsSection>,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders when isEmpty is false', () => {
    render(
      <AnalyticsSection title="Quality" isEmpty={false}>
        <div>content</div>
      </AnalyticsSection>,
    )
    expect(screen.getByText('Quality')).toBeInTheDocument()
  })
})
