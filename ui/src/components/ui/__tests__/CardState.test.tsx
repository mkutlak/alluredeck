import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CardState } from '../CardState'

type Props = Omit<Parameters<typeof CardState>[0], 'refetch' | 'children'>

function renderCardState(overrides: Partial<Props>) {
  const refetch = vi.fn()
  const { container } = render(
    <CardState isLoading={false} isError={false} isEmpty={false} refetch={refetch} {...overrides}>
      <span>content</span>
    </CardState>,
  )
  return { container, refetch }
}

describe('CardState', () => {
  // Precedence: loading > error > empty > content. Loading shows only
  // text-less skeletons.
  it.each<[string, Partial<Props>, string]>([
    [
      'loading wins over error and empty',
      { isLoading: true, isError: true, isEmpty: true, skeletonRows: 3 },
      '',
    ],
    [
      'error wins over empty',
      { isError: true, isEmpty: true, error: new Error('Server exploded') },
      "Couldn't load dataServer explodedRetry",
    ],
    [
      'empty with a custom message',
      { isEmpty: true, emptyMessage: 'Nothing here yet' },
      'Nothing here yet',
    ],
    ['empty with the default message', { isEmpty: true }, 'No data available'],
    ['content otherwise', {}, 'content'],
  ])('%s', (_, props, text) => {
    expect(renderCardState(props).container.textContent).toBe(text)
  })

  it('calls refetch when Retry button is clicked', async () => {
    const user = userEvent.setup()
    const { refetch } = renderCardState({ isError: true, error: new Error('oops') })
    await user.click(screen.getByRole('button', { name: /retry/i }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })
})
