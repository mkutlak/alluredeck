import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/projects', () => ({
  updateProjectTags: vi.fn(),
  getTags: vi.fn(),
}))
mockApiClient()

import { EditTagsDialog } from './EditTagsDialog'
import * as projectsApi from '@/api/projects'

function renderDialog(props: {
  projectId?: string
  currentTags?: string[]
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const qc = createTestQueryClient()
  vi.mocked(projectsApi.getTags).mockResolvedValue({
    data: ['backend', 'nightly', 'frontend'],
    metadata: { message: '' },
  })
  return render(
    <QueryClientProvider client={qc}>
      <EditTagsDialog
        projectId={props.projectId ?? 'test-proj'}
        currentTags={props.currentTags ?? []}
        open={props.open ?? true}
        onOpenChange={props.onOpenChange ?? vi.fn()}
      />
    </QueryClientProvider>,
  )
}

describe('EditTagsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders with existing tags', () => {
    renderDialog({ currentTags: ['backend', 'nightly'] })
    expect(screen.getByText('backend')).toBeInTheDocument()
    expect(screen.getByText('nightly')).toBeInTheDocument()
  })

  it('renders project name in title', () => {
    renderDialog({ projectId: 'my-proj' })
    expect(screen.getByText(/my-proj/)).toBeInTheDocument()
  })

  it('adds a tag when Enter is pressed', async () => {
    const user = userEvent.setup()
    renderDialog({ currentTags: [] })
    const input = screen.getByPlaceholderText(/type a tag/i)
    await user.type(input, 'new-tag{Enter}')
    expect(screen.getByText('new-tag')).toBeInTheDocument()
  })

  it('removes a tag when X is clicked', async () => {
    const user = userEvent.setup()
    renderDialog({ currentTags: ['removeme'] })
    const removeBtn = screen.getByLabelText('Remove tag removeme')
    await user.click(removeBtn)
    expect(screen.queryByText('removeme')).not.toBeInTheDocument()
  })

  it('shows validation error for invalid tag characters', async () => {
    const user = userEvent.setup()
    renderDialog({ currentTags: [] })
    const input = screen.getByPlaceholderText(/type a tag/i)
    await user.type(input, 'invalid tag!{Enter}')
    expect(screen.getByText(/invalid characters/i)).toBeInTheDocument()
  })

  it('submits correct payload on Save', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    vi.mocked(projectsApi.updateProjectTags).mockResolvedValue({
      data: { project_id: 'test-proj', tags: ['backend'] },
      metadata: { message: '' },
    })
    renderDialog({ currentTags: ['backend'], onOpenChange })
    const saveBtn = screen.getByRole('button', { name: /save tags/i })
    await user.click(saveBtn)
    await waitFor(() => {
      expect(projectsApi.updateProjectTags).toHaveBeenCalledWith('test-proj', ['backend'])
    })
  })

  it('closes when Cancel is clicked', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    renderDialog({ onOpenChange })
    const cancelBtn = screen.getByRole('button', { name: /cancel/i })
    await user.click(cancelBtn)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('syncs tags when currentTags prop changes', () => {
    vi.mocked(projectsApi.getTags).mockResolvedValue({ data: [], metadata: { message: '' } })
    const qc = createTestQueryClient()
    const onOpenChange = vi.fn()
    const { rerender } = render(
      <QueryClientProvider client={qc}>
        <EditTagsDialog
          projectId="test-proj"
          currentTags={['old-tag']}
          open={true}
          onOpenChange={onOpenChange}
        />
      </QueryClientProvider>,
    )
    expect(screen.getByText('old-tag')).toBeInTheDocument()
    rerender(
      <QueryClientProvider client={qc}>
        <EditTagsDialog
          projectId="test-proj"
          currentTags={['new-tag']}
          open={true}
          onOpenChange={onOpenChange}
        />
      </QueryClientProvider>,
    )
    expect(screen.getByText('new-tag')).toBeInTheDocument()
  })
})
