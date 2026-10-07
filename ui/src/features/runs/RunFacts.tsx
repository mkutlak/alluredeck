import { Fragment, useEffect, useReducer, type ReactNode } from 'react'
import { NavLink } from 'react-router'
import { useQuery } from '@tanstack/react-query'

import { formatDate } from '@/lib/utils'
import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import { formatProjectLabel } from '@/lib/projectLabel'
import { projectIndexOptions } from '@/lib/queries'
import { formatRelativeTime, summarizeRun } from './rowFormat'
import type { PipelineRun } from '@/types/api'

// Facts about a run that the feed row and the run page both show, written once.

/**
 * The run's headline, "2/9 suites failing · 14 failed tests", as parts joined by
 * " · ". `extra` appends parts (the run page adds pass rate and duration); later
 * parts such as "3 new · 1 inherited" or "awaiting N suites" slot in the same way.
 */
export function RunSummary({ run, extra = [] }: { run: PipelineRun; extra?: ReactNode[] }) {
  const { verdict, suitesFailing, failedCount } = summarizeRun(run)
  const total = run.aggregate.suites_total
  const parts: ReactNode[] =
    verdict === 'failed'
      ? [
          // Spell out "failing": a bare "7/8 suites" reads as 7 passing.
          `${suitesFailing}/${total} suites failing`,
          failedCount > 0 && (
            // The one red figure on the line; the glyph and this count carry status.
            <span className={`font-medium ${STATUS_TEXT_CLASSES.failed}`}>
              {`${failedCount} ${failedCount === 1 ? 'failed test' : 'failed tests'}`}
            </span>
          ),
        ]
      : verdict === 'skipped'
        ? ['all tests skipped']
        : [`${total}/${total} suites passed`]

  return [...parts, ...extra]
    .filter((part) => part !== false && part != null && part !== '')
    .map((part, i) => (
      <Fragment key={i}>
        {i > 0 && ' · '}
        {part}
      </Fragment>
    ))
}

// The label is a function of the clock and React Query will not re-render an unchanged
// run, so a page left open ticks on its own. ponytail: one timer per instance, lift into
// the feed page if pages ever hold hundreds of rows.
export function RelativeTime({ timestamp, className }: { timestamp: string; className: string }) {
  const [, tick] = useReducer((n: number) => n + 1, 0)
  useEffect(() => {
    const id = setInterval(tick, 60_000)
    return () => clearInterval(id)
  }, [])
  return (
    <time dateTime={timestamp} title={formatDate(timestamp)} className={className}>
      {formatRelativeTime(timestamp)}
    </time>
  )
}

/** The run's group by its hierarchical label, linked by numeric id when it has one. */
export function RunGroupLink({ run, className }: { run: PipelineRun; className: string }) {
  const { data } = useQuery(projectIndexOptions())
  const projects = data?.data
  const group =
    run.group_project_id != null
      ? projects?.find((p) => p.project_id === run.group_project_id)
      : undefined
  const label = group ? formatProjectLabel(group, projects) : run.group_slug
  if (!label) return null

  return run.group_project_id != null ? (
    <NavLink to={`/projects/${run.group_project_id}`} className={`${className} hover:underline`}>
      {label}
    </NavLink>
  ) : (
    <span className={className}>{label}</span>
  )
}
