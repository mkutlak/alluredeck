import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AnalyticsSection } from '../AnalyticsSection'

describe('AnalyticsSection', () => {
  it('renders title and children', () => {
    render(
      <AnalyticsSection title="Trends" isEmpty={false}>
        <div data-testid="child">content</div>
      </AnalyticsSection>,
    )
    expect(screen.getByRole('heading', { name: 'Trends' })).toBeInTheDocument()
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
})
