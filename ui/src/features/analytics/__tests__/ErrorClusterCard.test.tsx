import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { ErrorClusterCard } from '../ErrorClusterCard'
import * as analyticsApi from '@/api/analytics'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/analytics')
mockApiClient()

describe('ErrorClusterCard', () => {
  it('lists failure messages with counts, truncating long ones to 80 characters', async () => {
    const longMessage = 'A'.repeat(120)
    vi.mocked(analyticsApi.fetchTopErrors).mockResolvedValue({
      data: [
        { message: 'NullPointerException at com.example.Test', count: 42 },
        { message: longMessage, count: 7 },
      ],
      metadata: { message: 'ok' },
    })
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <ErrorClusterCard projectId="myproject" />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('NullPointerException at com.example.Test')).toBeInTheDocument()
    expect(screen.getByText(`${'A'.repeat(80)}...`)).toBeInTheDocument()
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
  })
})
