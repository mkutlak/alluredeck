import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { EnvironmentCard } from '../EnvironmentCard'
import * as reportsApi from '@/api/reports'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function renderCard() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <EnvironmentCard projectId="myproject" />
    </QueryClientProvider>,
  )
}

describe('EnvironmentCard', () => {
  it('renders environment entries', async () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockResolvedValue([
      { name: 'Browser', values: ['Chrome 120'] },
      { name: 'OS', values: ['Linux', 'macOS'] },
    ])
    renderCard()

    expect(await screen.findByText('Browser')).toBeInTheDocument()
    expect(screen.getByText('Chrome 120')).toBeInTheDocument()
    expect(screen.getByText('Linux, macOS')).toBeInTheDocument()
  })

  it('renders nothing when no entries', async () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockResolvedValue([])
    const { container } = renderCard()
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })
})
