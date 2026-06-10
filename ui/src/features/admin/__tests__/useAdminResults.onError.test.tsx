import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { mockApiClient } from '@/test/mocks/api-client'

// Mock API + toast before imports that reference them
vi.mock('@/api/admin')
vi.mock('@/components/ui/use-toast', () => ({
  toast: vi.fn(),
}))
mockApiClient()

import * as adminApi from '@/api/admin'
import { toast } from '@/components/ui/use-toast'
import { useAdminResults } from '../hooks/useAdminResults'

// Minimal component that exposes the hook's mutation
function Harness() {
  const { doClean } = useAdminResults()
  return (
    <button onClick={() => doClean('proj-1')}>clean</button>
  )
}

function renderHarness() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <Harness />
    </QueryClientProvider>,
  )
}

describe('useAdminResults onError toast', () => {
  const mockToast = vi.mocked(toast)

  beforeEach(() => {
    vi.clearAllMocks()
    // fetchAdminResults must resolve so the query doesn't stall
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])
  })

  it('shows destructive toast when doClean fails', async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.cleanAdminResults).mockRejectedValue(new Error('Storage unavailable'))

    renderHarness()
    await user.click(screen.getByRole('button', { name: 'clean' }))

    await waitFor(() => {
      expect(mockToast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Failed to clean results',
          variant: 'destructive',
        }),
      )
    })
  })

  it('does not show toast when doClean succeeds', async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.cleanAdminResults).mockResolvedValue(undefined)

    renderHarness()
    await user.click(screen.getByRole('button', { name: 'clean' }))

    await waitFor(() => {
      expect(mockToast).not.toHaveBeenCalled()
    })
  })
})
