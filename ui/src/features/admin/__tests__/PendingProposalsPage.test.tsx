import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Routes, Route } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { PendingProposalsPage } from '../PendingProposalsPage'
import * as proposalsApi from '@/api/proposals'
import * as systemApi from '@/api/system'
import { useAuthStore, type Role } from '@/store/auth'
import type { ConfigData } from '@/types/api'
import type { DefectProposal, FlakyProposal, KnownIssueProposal } from '@/types/proposals'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/proposals')
vi.mock('@/api/system')
mockApiClient()

function renderPage(roles: Role[] = ['admin']) {
  useAuthStore.setState({ roles })
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/admin/proposals']}>
        <Routes>
          <Route path="/admin/proposals" element={<PendingProposalsPage />} />
          <Route path="/" element={<div data-testid="dashboard" />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function mockConfig(mcpEnabled: boolean) {
  vi.mocked(systemApi.getConfig).mockResolvedValue({
    data: { mcp_enabled: mcpEnabled } as ConfigData,
    metadata: { message: 'OK' },
  })
}

function makeDefectProposal(overrides: Partial<DefectProposal> = {}): DefectProposal {
  return {
    id: 1,
    project_id: 1,
    proposer_user_id: 1,
    status: 'pending',
    created_at: '2026-05-01T10:00:00Z',
    fingerprint_hash: 'abcdef1234567890',
    proposed_category: 'product_bug',
    ...overrides,
  }
}

function makeKnownIssueProposal(overrides: Partial<KnownIssueProposal> = {}): KnownIssueProposal {
  return {
    id: 2,
    project_id: 1,
    proposer_user_id: 1,
    status: 'pending',
    created_at: '2026-05-01T10:00:00Z',
    error_message_sample: 'Connection refused',
    proposed_category: 'infrastructure',
    regex_pattern: 'Connection.*refused',
    applies_to_status: ['failed'],
    dry_run_match_count: 42,
    ...overrides,
  }
}

function makeFlakyProposal(overrides: Partial<FlakyProposal> = {}): FlakyProposal {
  return {
    id: 3,
    project_id: 1,
    proposer_user_id: 1,
    status: 'pending',
    created_at: '2026-05-01T10:00:00Z',
    test_full_name: 'Suite > flaky test name',
    history_id: 'hist-abc123',
    ...overrides,
  }
}

describe('PendingProposalsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockConfig(true)
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({ items: [], next_cursor: '' })
    vi.mocked(proposalsApi.listKnownIssueProposals).mockResolvedValue({
      items: [],
      next_cursor: '',
    })
    vi.mocked(proposalsApi.listFlakyProposals).mockResolvedValue({ items: [], next_cursor: '' })
  })

  it('redirects non-admin to dashboard', () => {
    renderPage([])
    expect(screen.getByTestId('dashboard')).toBeInTheDocument()
    expect(screen.queryByText('Pending Proposals')).not.toBeInTheDocument()
  })

  it('shows the page title and empty state for an admin with MCP enabled', async () => {
    renderPage()
    expect(screen.getByText('Pending Proposals')).toBeInTheDocument()
    expect(await screen.findByText(/No pending proposals/i)).toBeInTheDocument()
  })

  it('shows MCP disabled message when mcp_enabled is false', async () => {
    mockConfig(false)
    renderPage()
    expect(await screen.findByText(/MCP server is not enabled/i)).toBeInTheDocument()
  })

  it.each([
    {
      tab: /known issues/i,
      setup: () =>
        vi.mocked(proposalsApi.listKnownIssueProposals).mockResolvedValue({
          items: [makeKnownIssueProposal({ dry_run_match_count: 42 })],
          next_cursor: '',
        }),
      // dry_run_match_count is shown prominently.
      texts: ['42', /recent failures/i],
    },
    {
      tab: /flaky tests/i,
      setup: () =>
        vi.mocked(proposalsApi.listFlakyProposals).mockResolvedValue({
          items: [makeFlakyProposal({ test_full_name: 'Checkout > payment flow' })],
          next_cursor: '',
        }),
      texts: ['Checkout > payment flow'],
    },
  ])('lists proposals on the $tab tab', async ({ tab, setup, texts }) => {
    setup()
    renderPage()

    await userEvent.click(screen.getByRole('button', { name: tab }))
    for (const text of texts) expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it('clicking Approve opens confirmation dialog and calls mutation', async () => {
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({
      items: [makeDefectProposal({ id: 7 })],
      next_cursor: '',
    })
    vi.mocked(proposalsApi.approveProposal).mockResolvedValue()
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: /approve/i }))
    await userEvent.click(await screen.findByRole('button', { name: /^approve$/i }))

    await waitFor(() => {
      expect(proposalsApi.approveProposal).toHaveBeenCalledWith('defect', 7)
    })
  })

  it('clicking Reject opens dialog, requires reason, then calls mutation', async () => {
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({
      items: [makeDefectProposal({ id: 9 })],
      next_cursor: '',
    })
    vi.mocked(proposalsApi.rejectProposal).mockResolvedValue()
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: /reject/i }))
    const confirmBtn = await screen.findByRole('button', { name: /^reject$/i })
    expect(confirmBtn).toBeDisabled()

    await userEvent.type(screen.getByRole('textbox', { name: /rejection reason/i }), 'Not valid')
    expect(confirmBtn).not.toBeDisabled()
    await userEvent.click(confirmBtn)

    await waitFor(() => {
      expect(proposalsApi.rejectProposal).toHaveBeenCalledWith('defect', 9, {
        reason: 'Not valid',
      })
    })
  })

  it('shows Load more button when next_cursor is non-empty', async () => {
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({
      items: [makeDefectProposal()],
      next_cursor: 'cursor-abc',
    })
    renderPage()

    expect(await screen.findByRole('button', { name: /load more/i })).toBeInTheDocument()
  })
})
