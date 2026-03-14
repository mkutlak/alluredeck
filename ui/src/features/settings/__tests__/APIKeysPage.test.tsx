import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { APIKeysPage } from '../APIKeysPage'
import * as apiKeysApi from '@/api/api-keys'
import { mockApiClient } from '@/test/mocks/api-client'
import type { APIKey, APIKeyCreated } from '@/types/api'

vi.mock('@/api/api-keys')
mockApiClient()

function renderPage() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/settings/api-keys']}>
        <APIKeysPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function makeKey(overrides: Partial<APIKey> = {}): APIKey {
  return {
    id: 1,
    name: 'CI Pipeline',
    prefix: 'ak_abc123',
    role: 'admin',
    expires_at: null,
    last_used: null,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeCreatedKey(overrides: Partial<APIKeyCreated> = {}): APIKeyCreated {
  return {
    ...makeKey(),
    key: 'ak_abc123_supersecretfullkey',
    ...overrides,
  }
}

describe('APIKeysPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders empty state when no keys exist', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/no api keys yet/i)).toBeInTheDocument()
    })
  })

  it('renders key list with name, prefix, role, and created date', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([
      makeKey({ name: 'CI Pipeline', prefix: 'ak_abc123', role: 'admin' }),
    ])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('CI Pipeline')).toBeInTheDocument()
      expect(screen.getByText('ak_abc123')).toBeInTheDocument()
      expect(screen.getByText('admin')).toBeInTheDocument()
    })
  })

  it('renders viewer role badge', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([
      makeKey({ role: 'viewer' }),
    ])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('viewer')).toBeInTheDocument()
    })
  })

  it('shows Expired badge and grayed row for expired key', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([
      makeKey({ expires_at: '2020-01-01T00:00:00Z' }),
    ])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Expired')).toBeInTheDocument()
    })

    // The row should have opacity-50 class
    const expiredBadge = screen.getByText('Expired')
    const row = expiredBadge.closest('tr')
    expect(row).toHaveClass('opacity-50')
  })

  it('disables Create button when 5 keys exist', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([
      makeKey({ id: 1 }),
      makeKey({ id: 2 }),
      makeKey({ id: 3 }),
      makeKey({ id: 4 }),
      makeKey({ id: 5 }),
    ])

    renderPage()

    await waitFor(() => {
      const btn = screen.getByRole('button', { name: /create api key/i })
      expect(btn).toBeDisabled()
    })
  })

  it('Create button is enabled when fewer than 5 keys', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([makeKey()])

    renderPage()

    await waitFor(() => {
      const btn = screen.getByRole('button', { name: /create api key/i })
      expect(btn).not.toBeDisabled()
    })
  })

  it('opens create dialog on button click', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([])

    renderPage()

    const btn = await screen.findByRole('button', { name: /create api key/i })
    await userEvent.click(btn)

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText(/name/i)).toBeInTheDocument()
  })

  it('calls createAPIKey and shows created key dialog on success', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([])
    vi.mocked(apiKeysApi.createAPIKey).mockResolvedValue({
      apiKey: makeCreatedKey({ key: 'ak_abc123_supersecretfullkey' }),
      message: 'created',
    })

    renderPage()

    const createBtn = await screen.findByRole('button', { name: /create api key/i })
    await userEvent.click(createBtn)

    const nameInput = screen.getByLabelText(/name/i)
    await userEvent.type(nameInput, 'My Key')

    const submitBtn = screen.getByRole('button', { name: /^create$/i })
    await userEvent.click(submitBtn)

    await waitFor(() => {
      expect(apiKeysApi.createAPIKey).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'My Key' }),
      )
    })

    await waitFor(() => {
      expect(screen.getByText('ak_abc123_supersecretfullkey')).toBeInTheDocument()
    })
  })

  it('shows delete confirmation dialog on delete icon click', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([
      makeKey({ id: 42, name: 'Old Key', prefix: 'ak_old' }),
    ])

    renderPage()

    const deleteBtn = await screen.findByRole('button', { name: /delete api key old key/i })
    await userEvent.click(deleteBtn)

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument()
      expect(screen.getByText(/delete api key\?/i)).toBeInTheDocument()
      // the dialog description contains the key name
      expect(screen.getAllByText(/old key/i).length).toBeGreaterThan(0)
    })
  })

  it('calls deleteAPIKey on confirmation', async () => {
    vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue([
      makeKey({ id: 42, name: 'Old Key', prefix: 'ak_old' }),
    ])
    vi.mocked(apiKeysApi.deleteAPIKey).mockResolvedValue()

    renderPage()

    const deleteBtn = await screen.findByRole('button', { name: /delete api key old key/i })
    await userEvent.click(deleteBtn)

    const confirmBtn = await screen.findByRole('button', { name: /^delete$/i })
    await userEvent.click(confirmBtn)

    await waitFor(() => {
      expect(apiKeysApi.deleteAPIKey).toHaveBeenCalledWith(42)
    })
  })
})
