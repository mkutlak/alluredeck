import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReportPagination } from '../ReportPagination'

function renderPagination(page: number, totalPages: number, onPerPageChange = vi.fn()) {
  render(
    <ReportPagination
      page={page}
      totalPages={totalPages}
      onPageChange={vi.fn()}
      perPage={20}
      onPerPageChange={onPerPageChange}
    />,
  )
  return onPerPageChange
}

describe('ReportPagination', () => {
  it.each([
    { page: 1, totalPages: 3, prevDisabled: true, nextDisabled: false },
    { page: 2, totalPages: 3, prevDisabled: false, nextDisabled: false },
    { page: 3, totalPages: 3, prevDisabled: false, nextDisabled: true },
  ])(
    'on page $page of $totalPages disables previous=$prevDisabled, next=$nextDisabled',
    ({ page, totalPages, prevDisabled, nextDisabled }) => {
      renderPagination(page, totalPages)

      expect(screen.getByText(`Page ${page} of ${totalPages}`)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /previous/i }).hasAttribute('disabled')).toBe(
        prevDisabled,
      )
      expect(screen.getByRole('button', { name: /next/i }).hasAttribute('disabled')).toBe(
        nextDisabled,
      )
    },
  )

  it('shows the rows per page and reports a numeric choice', async () => {
    const user = userEvent.setup()
    const onPerPageChange = renderPagination(1, 3)
    const select = screen.getByRole('combobox', { name: /rows per page/i })
    expect(select).toHaveTextContent('20')

    await user.click(select)
    await user.click(await screen.findByRole('option', { name: '50' }))

    expect(onPerPageChange).toHaveBeenCalledWith(50)
  })
})
