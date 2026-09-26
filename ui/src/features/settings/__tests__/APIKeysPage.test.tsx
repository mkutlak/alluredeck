import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
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

function renderPage(keys: APIKey[]) {
  vi.mocked(apiKeysApi.fetchAPIKeys).mockResolvedValue(keys)
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
    allow_mcp_writes: false,
    expires_at: null,
    last_used: null,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeCreatedKey(overrides: Partial<APIKeyCreated> = {}): APIKeyCreated {
  return { ...makeKey(), key: 'ak_abc123_supersecretfullkey', ...overrides }
}

describe('APIKeysPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders empty state when no keys exist', async () => {
    renderPage([])
    expect(await screen.findByText(/no api keys yet/i)).toBeInTheDocument()
  })

  it('lists keys with prefix, role, and Expired/MCP badges on the matching row only', async () => {
    renderPage([
      makeKey({ id: 1, name: 'CI Pipeline', prefix: 'ak_abc123', role: 'admin' }),
      makeKey({
        id: 2,
        name: 'Old MCP Key',
        prefix: 'ak_old',
        role: 'viewer',
        allow_mcp_writes: true,
        expires_at: '2020-01-01T00:00:00Z',
      }),
    ])

    const active = within((await screen.findByText('CI Pipeline')).closest('tr')!)
    const expired = within(screen.getByText('Old MCP Key').closest('tr')!)
    expect(active.getByText('ak_abc123')).toBeInTheDocument()
    expect(active.getByText('admin')).toBeInTheDocument()
    expect(active.queryByText('Expired')).not.toBeInTheDocument()
    expect(active.queryByText('MCP')).not.toBeInTheDocument()
    expect(expired.getByText('viewer')).toBeInTheDocument()
    expect(expired.getByText('Expired')).toBeInTheDocument()
    expect(expired.getByText('MCP')).toBeInTheDocument()
  })

  it.each([
    { count: 5, disabled: true },
    { count: 1, disabled: false },
  ])('Create button disabled=$disabled with $count keys (limit 5)', async ({ count, disabled }) => {
    renderPage(Array.from({ length: count }, (_, i) => makeKey({ id: i + 1 })))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /create api key/i }).hasAttribute('disabled')).toBe(
        disabled,
      )
    })
  })

  it.each([
    { name: 'My Key', mcp: false },
    { name: 'MCP Key', mcp: true },
  ])(
    'creates a key with allow_mcp_writes=$mcp and shows the full key once',
    async ({ name, mcp }) => {
      const user = userEvent.setup()
      vi.mocked(apiKeysApi.createAPIKey).mockResolvedValue({
        apiKey: makeCreatedKey({ allow_mcp_writes: mcp }),
        message: 'created',
      })
      renderPage([])

      await user.click(await screen.findByRole('button', { name: /create api key/i }))
      await user.type(screen.getByLabelText(/name/i), name)
      if (mcp) await user.click(screen.getByRole('checkbox', { name: /allow mcp writes/i }))
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      await waitFor(() => {
        expect(apiKeysApi.createAPIKey).toHaveBeenCalledWith(
          expect.objectContaining({ name, allow_mcp_writes: mcp }),
        )
      })
      expect(await screen.findByText('ak_abc123_supersecretfullkey')).toBeInTheDocument()
    },
  )

  it('confirms deletion naming the key, then calls deleteAPIKey', async () => {
    const user = userEvent.setup()
    vi.mocked(apiKeysApi.deleteAPIKey).mockResolvedValue()
    renderPage([makeKey({ id: 42, name: 'Old Key', prefix: 'ak_old' })])

    await user.click(await screen.findByRole('button', { name: /delete api key old key/i }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/delete api key\?/i)).toBeInTheDocument()
    expect(within(dialog).getByText(/old key/i)).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: /^delete$/i }))
    await waitFor(() => {
      expect(apiKeysApi.deleteAPIKey).toHaveBeenCalledWith(42)
    })
  })
})
