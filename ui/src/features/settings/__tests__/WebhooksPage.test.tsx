import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { WebhooksPage } from '../WebhooksPage'
import * as webhooksApi from '@/api/webhooks'
import * as projectsApi from '@/api/projects'
import { mockApiClient } from '@/test/mocks/api-client'
import type { Webhook } from '@/types/api'

vi.mock('@/api/webhooks')
vi.mock('@/api/projects')
mockApiClient()

function renderPage(webhooks: Webhook[] = [], search = '?project=1') {
  vi.mocked(webhooksApi.fetchWebhooks).mockResolvedValue(webhooks)
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={[`/settings/webhooks${search}`]}>
        <WebhooksPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function makeWebhook(overrides: Partial<Webhook> = {}): Webhook {
  return {
    id: 'wh-1',
    project_id: 1,
    name: 'CI Alerts',
    target_type: 'slack',
    url: 'https://hooks.slack.com/services/abc123',
    has_secret: false,
    template: null,
    events: ['report.generated'],
    is_active: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('WebhooksPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: [
        { project_id: 1, slug: 'my-project', display_name: 'My Project', parent_id: null },
        { project_id: 2, slug: 'other-project', display_name: 'Other Project', parent_id: null },
      ],
      metadata: { message: 'ok' },
    })
  })

  it.each([
    { name: 'the empty state', webhooks: [], texts: [/no webhooks yet/i] },
    {
      name: 'webhook names',
      webhooks: [
        makeWebhook({ id: 'wh-1', name: 'CI Alerts', target_type: 'slack' }),
        makeWebhook({ id: 'wh-2', name: 'Discord Notifier', target_type: 'discord' }),
      ],
      texts: ['CI Alerts', 'Discord Notifier'],
    },
  ])('lists the selected project webhooks: $name', async ({ webhooks, texts }) => {
    renderPage(webhooks)
    for (const text of texts) expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it.each([
    { search: '', picker: /select a project\.\.\./i, prompt: true },
    { search: '?project=1', picker: 'My Project', prompt: false },
  ])('labels the project picker for search $search', async ({ search, picker, prompt }) => {
    renderPage([], search)

    expect(await screen.findByRole('button', { name: picker })).toBeInTheDocument()
    expect(screen.queryByText(/select a project to manage its webhooks/i) !== null).toBe(prompt)
    expect(screen.queryByRole('button', { name: /add webhook/i }) !== null).toBe(!prompt)
  })

  it('calls createWebhook with form values on submit', async () => {
    const user = userEvent.setup()
    vi.mocked(webhooksApi.createWebhook).mockResolvedValue(
      makeWebhook({ name: 'New Hook', url: 'https://example.com/hook' }),
    )
    renderPage()

    await user.click(await screen.findByRole('button', { name: /add webhook/i }))
    await user.clear(screen.getByLabelText(/^name$/i))
    await user.type(screen.getByLabelText(/^name$/i), 'New Hook')
    await user.clear(screen.getByLabelText(/^url$/i))
    await user.type(screen.getByLabelText(/^url$/i), 'https://x.co/h')
    await user.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => {
      expect(webhooksApi.createWebhook).toHaveBeenCalledWith(
        '1',
        expect.objectContaining({ name: 'New Hook', url: 'https://x.co/h' }),
      )
    })
  }, 10000)

  it('calls deleteWebhook when delete is confirmed', async () => {
    const user = userEvent.setup()
    vi.mocked(webhooksApi.deleteWebhook).mockResolvedValue()
    renderPage([makeWebhook({ id: 'wh-42', name: 'Old Hook' })])

    await user.click(await screen.findByRole('button', { name: /delete webhook old hook/i }))
    expect(await screen.findByText(/delete webhook\?/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(webhooksApi.deleteWebhook).toHaveBeenCalledWith('1', 'wh-42')
    })
  })
})
