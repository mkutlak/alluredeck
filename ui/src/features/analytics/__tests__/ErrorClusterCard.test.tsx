import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { ErrorClusterCard } from '../ErrorClusterCard'
import * as analyticsApi from '@/api/analytics'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/analytics')
mockApiClient()

function renderCard(projectId = 'myproject') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <ErrorClusterCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('ErrorClusterCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows title while loading', () => {
    vi.mocked(analyticsApi.fetchTopErrors).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Top Failure Messages')).toBeInTheDocument()
  })

  it('renders table with error data', async () => {
    vi.mocked(analyticsApi.fetchTopErrors).mockResolvedValue({
      data: [
        { message: 'NullPointerException at com.example.Test', count: 42 },
        { message: 'AssertionError: expected true to be false', count: 7 },
      ],
      metadata: { message: 'ok' },
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('NullPointerException at com.example.Test')).toBeInTheDocument()
    })
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
  })

  it('truncates long messages to 80 characters', async () => {
    const longMessage = 'A'.repeat(120)
    vi.mocked(analyticsApi.fetchTopErrors).mockResolvedValue({
      data: [{ message: longMessage, count: 1 }],
      metadata: { message: 'ok' },
    })
    renderCard()
    await waitFor(() => {
      const truncated = longMessage.slice(0, 80) + '...'
      expect(screen.getByText(truncated)).toBeInTheDocument()
    })
  })

  it('shows placeholder when data is empty', async () => {
    vi.mocked(analyticsApi.fetchTopErrors).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('No failure data available')).toBeInTheDocument()
    })
  })
})
