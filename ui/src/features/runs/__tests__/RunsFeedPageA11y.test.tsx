import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunsFeedPage } from '../RunsFeedPage'
import { useUIStore } from '@/store/ui'
import type { PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunsFeed: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunFailures: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', parent_id: null, children: [1] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/branches', () => ({ fetchBranches: vi.fn().mockResolvedValue([]) }))

import { fetchRunsFeed } from '@/api/pipeline'

function makeRun(pipeline_id: string, branch: string): PipelineRun {
  return {
    pipeline_id,
    commit_sha: `sha${pipeline_id}`,
    branch,
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [
      {
        project_id: 1,
        slug: 'api-cloud',
        build_number: 5,
        build_id: 105,
        pass_rate: 100,
        total: 42,
        failed: 0,
        duration_ms: 15000,
        status: 'passed',
        builds: [{ build_id: 105, build_number: 5 }],
      },
    ],
    aggregate: {
      suites_passed: 1,
      suites_total: 1,
      tests_passed: 42,
      tests_total: 42,
      pass_rate: 100,
      total_duration_ms: 15000,
    },
  }
}

const runs = [makeRun('101', 'main'), makeRun('102', 'feature/login')]

async function renderFeed(data = runs) {
  vi.mocked(fetchRunsFeed).mockResolvedValue({
    data,
    metadata: { message: 'ok' },
    pagination: { page: 1, per_page: 10, total: data.length, total_pages: 1 },
  })
  const view = renderWithProviders(<RunsFeedPage />)
  await screen.findByRole('link', { name: 'main, run 101' })
  return view
}

beforeEach(() => {
  useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
  document.title = 'AllureDeck'
})

describe('RunsFeedPage for keyboard and screen-reader users', () => {
  // A screen reader announces "list, 2 items" and can jump run to run by heading.
  it('lists the runs, each headed by its branch link, under the one page heading', async () => {
    await renderFeed()

    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(
      items.map((item) => within(item).getByRole('heading', { level: 2 }).textContent),
    ).toEqual(['main', 'feature/login'])
    expect(
      within(within(items[1]!).getByRole('heading', { level: 2 })).getByRole('link', {
        name: 'feature/login, run 102',
      }),
    ).toBeInTheDocument()
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
  })

  it('names the tab "Runs" while the feed is open', async () => {
    const { unmount } = await renderFeed()
    expect(document.title).toBe('Runs — AllureDeck')
    unmount()
    expect(document.title).toBe('AllureDeck')
  })

  it('moves between runs with j and k once focus is in the list', async () => {
    const user = userEvent.setup()
    await renderFeed()

    await user.keyboard('j')
    expect(document.body).toHaveFocus()
    screen.getByRole('link', { name: 'main, run 101' }).focus()
    await user.keyboard('j')
    expect(screen.getByRole('link', { name: 'feature/login, run 102' })).toHaveFocus()
    await user.keyboard('k')
    expect(screen.getByRole('link', { name: 'main, run 101' })).toHaveFocus()
  })

  // A run outside any group has no page, so no branch link: j lands on its toggle.
  it('stops on the toggle of a failing run that has no branch link', async () => {
    const user = userEvent.setup()
    const orphan = makeRun('103', 'hotfix')
    orphan.group_project_id = undefined
    orphan.group_slug = 'orphan'
    orphan.suites = orphan.suites.map((s) => ({ ...s, failed: 2, status: 'degraded' }))
    await renderFeed([runs[0]!, orphan])

    screen.getByRole('link', { name: 'main, run 101' }).focus()
    await user.keyboard('j')
    expect(screen.getByRole('button', { name: 'Toggle failures for 103' })).toHaveFocus()
  })
})
