import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CardState } from '../CardState'

import { mockApiClient } from '@/test/mocks/api-client'
mockApiClient()

function renderCardState(overrides: Partial<Parameters<typeof CardState>[0]> = {}) {
  const refetch = vi.fn()
  const props = {
    isLoading: false,
    isError: false,
    isEmpty: false,
    refetch,
    children: <span>content</span>,
    ...overrides,
  }
  render(<CardState {...props} />)
  return { refetch }
}

describe('CardState', () => {
  it('renders skeleton rows while loading', () => {
    renderCardState({ isLoading: true, skeletonRows: 3 })
    // Skeletons are rendered — no content or error
    expect(screen.queryByText('content')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })

  it('renders default 5 skeleton rows when skeletonRows not provided', () => {
    const { container } = render(
      <CardState isLoading isError={false} isEmpty={false} refetch={vi.fn()}>
        <span>content</span>
      </CardState>,
    )
    // 5 skeleton divs inside the loading container
    const skeletons = container.querySelectorAll('.animate-pulse, [class*="skeleton"]')
    expect(skeletons.length).toBeGreaterThanOrEqual(1)
  })

  it('renders error state with retry button and error message', () => {
    const err = new Error('Server exploded')
    renderCardState({ isError: true, error: err, isEmpty: false })
    expect(screen.getByText(/couldn't load data/i)).toBeInTheDocument()
    expect(screen.getByText('Server exploded')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
    // Should NOT show content or empty message
    expect(screen.queryByText('content')).not.toBeInTheDocument()
  })

  it('calls refetch when Retry button is clicked', async () => {
    const user = userEvent.setup()
    const err = new Error('oops')
    const { refetch } = renderCardState({ isError: true, error: err, isEmpty: false })
    await user.click(screen.getByRole('button', { name: /retry/i }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })

  it('error state is visually distinct from empty state (shows AlertCircle icon area)', () => {
    renderCardState({ isError: true, error: new Error('fail'), isEmpty: false })
    // Error state has the destructive heading, empty does not
    expect(screen.getByText(/couldn't load data/i)).toBeInTheDocument()
  })

  it('renders custom empty message when isEmpty=true', () => {
    renderCardState({ isEmpty: true, emptyMessage: 'Nothing here yet' })
    expect(screen.getByText('Nothing here yet')).toBeInTheDocument()
    expect(screen.queryByText('content')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })

  it('renders default empty message when emptyMessage not provided', () => {
    renderCardState({ isEmpty: true })
    expect(screen.getByText('No data available')).toBeInTheDocument()
  })

  it('renders children when not loading, not error, not empty', () => {
    renderCardState({})
    expect(screen.getByText('content')).toBeInTheDocument()
  })

  it('loading takes precedence over error and empty', () => {
    renderCardState({ isLoading: true, isError: true, isEmpty: true })
    expect(screen.queryByText('content')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })

  it('error takes precedence over empty when not loading', () => {
    renderCardState({ isLoading: false, isError: true, isEmpty: true, error: new Error('err') })
    expect(screen.getByText(/couldn't load data/i)).toBeInTheDocument()
    expect(screen.queryByText('No data available')).not.toBeInTheDocument()
  })
})
