import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { EnvironmentCard } from '../EnvironmentCard'
import * as reportsApi from '@/api/reports'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function renderCard(projectId = 'myproject') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <EnvironmentCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('EnvironmentCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeletons initially', () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Environment')).toBeInTheDocument()
  })

  it('renders environment entries', async () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockResolvedValue([
      { name: 'Browser', values: ['Chrome 120'] },
      { name: 'OS', values: ['Linux', 'macOS'] },
    ])
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('Browser')).toBeInTheDocument()
      expect(screen.getByText('Chrome 120')).toBeInTheDocument()
      expect(screen.getByText('Linux, macOS')).toBeInTheDocument()
    })
  })

  it('renders nothing when no entries', async () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockResolvedValue([])
    const { container } = renderCard()
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })
})
