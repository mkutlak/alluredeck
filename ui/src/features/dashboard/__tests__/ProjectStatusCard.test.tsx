import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import type { DashboardProjectEntry } from '@/types/api'

vi.mock('recharts', () => ({
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  LineChart: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Line: () => null,
}))

import { ProjectStatusCard } from '../ProjectStatusCard'

function renderCard(project: DashboardProjectEntry) {
  return render(
    <MemoryRouter>
      <ProjectStatusCard project={project} />
    </MemoryRouter>,
  )
}

const healthyProject: DashboardProjectEntry = {
  project_id: 'my-project',
  created_at: '2025-01-01T00:00:00Z',
  latest_build: {
    build_order: 10,
    created_at: '2025-03-01T12:00:00Z',
    statistics: { passed: 95, failed: 2, broken: 1, skipped: 2, unknown: 0, total: 100 },
    pass_rate: 95.0,
    duration_ms: 60000,
    flaky_count: 0,
    new_failed_count: 0,
    new_passed_count: 1,
    ci_branch: 'main',
  },
  sparkline: [{ build_order: 9, pass_rate: 90 }, { build_order: 10, pass_rate: 95 }],
}

const noBuildsProject: DashboardProjectEntry = {
  project_id: 'empty-project',
  created_at: '2025-01-01T00:00:00Z',
  latest_build: null,
  sparkline: [],
}

describe('ProjectStatusCard', () => {
  it('displays project name', () => {
    renderCard(healthyProject)
    expect(screen.getByText('my-project')).toBeInTheDocument()
  })

  it('shows pass rate badge for healthy project', () => {
    renderCard(healthyProject)
    expect(screen.getByText('95%')).toBeInTheDocument()
  })

  it('shows "No builds" when latest_build is null', () => {
    renderCard(noBuildsProject)
    expect(screen.getByText(/no builds/i)).toBeInTheDocument()
  })

  it('shows CI branch when available', () => {
    renderCard(healthyProject)
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('renders a link to the project', () => {
    renderCard(healthyProject)
    const link = screen.getByRole('link', { name: /view project/i })
    expect(link).toBeInTheDocument()
    expect(link.getAttribute('href')).toBe('/projects/my-project')
  })
})
