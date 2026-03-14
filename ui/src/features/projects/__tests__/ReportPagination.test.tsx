import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReportPagination } from '../ReportPagination'

function renderPagination(props: {
  page?: number
  totalPages?: number
  onPageChange?: (updater: (p: number) => number) => void
  perPage?: number
  onPerPageChange?: (perPage: number) => void
} = {}) {
  const defaults = {
    page: 1,
    totalPages: 3,
    onPageChange: vi.fn(),
    perPage: 20,
    onPerPageChange: vi.fn(),
  }
  const merged = { ...defaults, ...props }
  return { ...render(<ReportPagination {...merged} />), ...merged }
}

describe('ReportPagination', () => {
  it('renders page info text', () => {
    renderPagination({ page: 2, totalPages: 5 })
    expect(screen.getByText(/page 2 of 5/i)).toBeInTheDocument()
  })

  it('Previous button is disabled on page 1', () => {
    renderPagination({ page: 1 })
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled()
  })

  it('Previous button is enabled on page 2', () => {
    renderPagination({ page: 2 })
    expect(screen.getByRole('button', { name: /previous/i })).not.toBeDisabled()
  })

  it('Next button is disabled on the last page', () => {
    renderPagination({ page: 3, totalPages: 3 })
    expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
  })

  it('Next button is enabled when not on last page', () => {
    renderPagination({ page: 1, totalPages: 3 })
    expect(screen.getByRole('button', { name: /next/i })).not.toBeDisabled()
  })

  it('renders per-page selector with current value', () => {
    renderPagination({ perPage: 20 })
    expect(screen.getByRole('combobox', { name: /rows per page/i })).toHaveTextContent('20')
  })

  it('per-page selector shows the provided perPage value', () => {
    renderPagination({ perPage: 50 })
    expect(screen.getByRole('combobox', { name: /rows per page/i })).toHaveTextContent('50')
  })

  it('onPerPageChange is called with numeric value when an option is selected', async () => {
    const user = userEvent.setup()
    const onPerPageChange = vi.fn()
    renderPagination({ perPage: 20, onPerPageChange })

    await user.click(screen.getByRole('combobox', { name: /rows per page/i }))
    const option = await screen.findByRole('option', { name: '50' })
    await user.click(option)

    expect(onPerPageChange).toHaveBeenCalledWith(50)
  })

  it('onPageChange is called when Next is clicked', async () => {
    const user = userEvent.setup()
    const onPageChange = vi.fn()
    renderPagination({ page: 1, totalPages: 3, onPageChange })

    await user.click(screen.getByRole('button', { name: /next/i }))
    expect(onPageChange).toHaveBeenCalledWith(expect.any(Function))
  })

  it('onPageChange is called when Previous is clicked', async () => {
    const user = userEvent.setup()
    const onPageChange = vi.fn()
    renderPagination({ page: 2, totalPages: 3, onPageChange })

    await user.click(screen.getByRole('button', { name: /previous/i }))
    expect(onPageChange).toHaveBeenCalledWith(expect.any(Function))
  })
})
