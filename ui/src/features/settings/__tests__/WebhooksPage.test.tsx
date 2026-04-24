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

function renderPage(search = '?project=1') {
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

  it('renders empty state when no webhooks configured', async () => {
    vi.mocked(webhooksApi.fetchWebhooks).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/no webhooks yet/i)).toBeInTheDocument()
    })
  })

  it('renders webhooks list with webhook names', async () => {
    vi.mocked(webhooksApi.fetchWebhooks).mockResolvedValue([
      makeWebhook({ id: 'wh-1', name: 'CI Alerts', target_type: 'slack' }),
      makeWebhook({ id: 'wh-2', name: 'Discord Notifier', target_type: 'discord' }),
    ])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('CI Alerts')).toBeInTheDocument()
      expect(screen.getByText('Discord Notifier')).toBeInTheDocument()
    })
  })

  it('shows project selector prompt when no project in URL', async () => {
    renderPage('')

    await waitFor(() => {
      expect(screen.getByText(/select a project to manage its webhooks/i)).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: /select a project\.\.\./i })).toBeInTheDocument()
  })

  it('renders project picker button with selected project display name', async () => {
    vi.mocked(webhooksApi.fetchWebhooks).mockResolvedValue([])

    renderPage('?project=1')

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'My Project' })).toBeInTheDocument()
    })
  })

  it('opens create dialog when add webhook button is clicked', async () => {
    vi.mocked(webhooksApi.fetchWebhooks).mockResolvedValue([])

    renderPage()

    const btn = await screen.findByRole('button', { name: /add webhook/i })
    await userEvent.click(btn)

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText(/name/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/url/i)).toBeInTheDocument()
  })

  it('calls createWebhook with form values on submit', async () => {
    vi.mocked(webhooksApi.fetchWebhooks).mockResolvedValue([])
    vi.mocked(webhooksApi.createWebhook).mockResolvedValue(
      makeWebhook({ name: 'New Hook', url: 'https://example.com/hook' }),
    )

    renderPage()

    const addBtn = await screen.findByRole('button', { name: /add webhook/i })
    await userEvent.click(addBtn)

    const nameInput = screen.getByLabelText(/^name$/i)
    await userEvent.clear(nameInput)
    await userEvent.type(nameInput, 'New Hook')

    const urlInput = screen.getByLabelText(/^url$/i)
    await userEvent.clear(urlInput)
    await userEvent.type(urlInput, 'https://x.co/h')

    const submitBtn = screen.getByRole('button', { name: /create/i })
    await userEvent.click(submitBtn)

    await waitFor(() => {
      expect(webhooksApi.createWebhook).toHaveBeenCalledWith(
        '1',
        expect.objectContaining({
          name: 'New Hook',
          url: 'https://x.co/h',
        }),
      )
    })
  }, 10000)

  it('calls deleteWebhook when delete is confirmed', async () => {
    vi.mocked(webhooksApi.fetchWebhooks).mockResolvedValue([
      makeWebhook({ id: 'wh-42', name: 'Old Hook' }),
    ])
    vi.mocked(webhooksApi.deleteWebhook).mockResolvedValue()

    renderPage()

    const deleteBtn = await screen.findByRole('button', { name: /delete webhook old hook/i })
    await userEvent.click(deleteBtn)

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument()
      expect(screen.getByText(/delete webhook\?/i)).toBeInTheDocument()
    })

    const confirmBtn = screen.getByRole('button', { name: /^delete$/i })
    await userEvent.click(confirmBtn)

    await waitFor(() => {
      expect(webhooksApi.deleteWebhook).toHaveBeenCalledWith('1', 'wh-42')
    })
  })
})
