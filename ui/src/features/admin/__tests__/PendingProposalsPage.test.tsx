import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Routes, Route } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { PendingProposalsPage } from '../PendingProposalsPage'
import * as proposalsApi from '@/api/proposals'
import * as systemApi from '@/api/system'
import { useAuthStore } from '@/store/auth'
import type { DefectProposal, FlakyProposal, KnownIssueProposal } from '@/types/proposals'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
}))
vi.mock('@/api/proposals')
vi.mock('@/api/system')
mockApiClient()

import type { AuthState } from '@/store/auth'

type AuthSelector = (s: Partial<AuthState>) => unknown

function renderPage(initialPath = '/admin/proposals', isAdmin = true) {
  vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
    (selector as AuthSelector)({ roles: isAdmin ? ['admin'] : [] }),
  )
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path="/admin/proposals" element={<PendingProposalsPage />} />
          <Route path="/" element={<div data-testid="dashboard" />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function makeConfig(mcpEnabled = true) {
  return {
    data: {
      version: '1.0.0',
      dev_mode: false,
      check_results_every_seconds: '30',
      keep_history: true,
      keep_history_latest: 20,
      tls: false,
      security_enabled: true,
      url_prefix: '',
      api_response_less_verbose: false,
      optimize_storage: false,
      make_viewer_endpoints_public: false,
      oidc_enabled: false,
      mcp_enabled: mcpEnabled,
    },
    metadata: { message: 'OK' },
  }
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

function makeKnownIssueProposal(
  overrides: Partial<KnownIssueProposal> = {},
): KnownIssueProposal {
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
    vi.mocked(systemApi.getConfig).mockResolvedValue(makeConfig(true))
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({
      items: [],
      next_cursor: '',
    })
    vi.mocked(proposalsApi.listKnownIssueProposals).mockResolvedValue({
      items: [],
      next_cursor: '',
    })
    vi.mocked(proposalsApi.listFlakyProposals).mockResolvedValue({
      items: [],
      next_cursor: '',
    })
  })

  it('redirects non-admin to dashboard', () => {
    renderPage('/admin/proposals', false)
    expect(screen.getByTestId('dashboard')).toBeInTheDocument()
    expect(screen.queryByText('Pending Proposals')).not.toBeInTheDocument()
  })

  it('shows MCP disabled message when mcp_enabled is false', async () => {
    vi.mocked(systemApi.getConfig).mockResolvedValue(makeConfig(false))
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/MCP server is not enabled/i)).toBeInTheDocument()
    })
  })

  it('renders page title for admin with mcp enabled', async () => {
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('Pending Proposals')).toBeInTheDocument()
    })
  })

  it('shows empty state when no defect proposals', async () => {
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/No pending proposals/i)).toBeInTheDocument()
    })
  })

  it('renders defect proposal rows when data present', async () => {
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({
      items: [
        makeDefectProposal({ id: 1, fingerprint_hash: 'abc123def456', proposed_category: 'test_bug' }),
        makeDefectProposal({ id: 2, fingerprint_hash: 'xyz999aaa111', proposed_category: 'product_bug' }),
      ],
      next_cursor: '',
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('test_bug')).toBeInTheDocument()
      expect(screen.getByText('product_bug')).toBeInTheDocument()
    })
  })

  it('shows known issue dry_run_match_count prominently', async () => {
    vi.mocked(proposalsApi.listKnownIssueProposals).mockResolvedValue({
      items: [makeKnownIssueProposal({ dry_run_match_count: 42 })],
      next_cursor: '',
    })
    renderPage()

    // Switch to Known Issues tab
    const knownIssueTab = screen.getByRole('button', { name: /known issues/i })
    await userEvent.click(knownIssueTab)

    await waitFor(() => {
      expect(screen.getByText('42')).toBeInTheDocument()
      expect(screen.getByText(/recent failures/i)).toBeInTheDocument()
    })
  })

  it('clicking Approve opens confirmation dialog and calls mutation', async () => {
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({
      items: [makeDefectProposal({ id: 7 })],
      next_cursor: '',
    })
    vi.mocked(proposalsApi.approveProposal).mockResolvedValue()

    renderPage()

    const approveBtn = await screen.findByRole('button', { name: /approve/i })
    await userEvent.click(approveBtn)

    const confirmBtn = await screen.findByRole('button', { name: /^approve$/i })
    await userEvent.click(confirmBtn)

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

    const rejectBtn = await screen.findByRole('button', { name: /reject/i })
    await userEvent.click(rejectBtn)

    // Confirm button should be disabled with no reason
    const confirmBtn = await screen.findByRole('button', { name: /^reject$/i })
    expect(confirmBtn).toBeDisabled()

    // Type a reason
    const input = screen.getByRole('textbox', { name: /rejection reason/i })
    await userEvent.type(input, 'Not valid')

    expect(confirmBtn).not.toBeDisabled()
    await userEvent.click(confirmBtn)

    await waitFor(() => {
      expect(proposalsApi.rejectProposal).toHaveBeenCalledWith('defect', 9, {
        reason: 'Not valid',
      })
    })
  })

  it('shows flaky test proposals on Flaky Tests tab', async () => {
    vi.mocked(proposalsApi.listFlakyProposals).mockResolvedValue({
      items: [makeFlakyProposal({ test_full_name: 'Checkout > payment flow' })],
      next_cursor: '',
    })
    renderPage()

    const flakyTab = screen.getByRole('button', { name: /flaky tests/i })
    await userEvent.click(flakyTab)

    await waitFor(() => {
      expect(screen.getByText('Checkout > payment flow')).toBeInTheDocument()
    })
  })

  it('shows Load more button when next_cursor is non-empty', async () => {
    vi.mocked(proposalsApi.listDefectProposals).mockResolvedValue({
      items: [makeDefectProposal()],
      next_cursor: 'cursor-abc',
    })
    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /load more/i })).toBeInTheDocument()
    })
  })
})
