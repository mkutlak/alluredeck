import { NavLink } from 'react-router'
import { Split } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import { suiteReportHref } from './runKey'
import type { PipelineSuite } from '@/types/api'

export interface RunSuiteChipProps {
  suite: PipelineSuite
  /** The run's page; the chip opens this suite's section there. Without it, the report. */
  runPath?: string
}

const STATUS_ICON = {
  passed: '✓',
  degraded: '⚠',
  failed: '✗',
  skipped: '○', // nothing ran: neither ✓ nor ✗ is true
} as const

// Suite status is derived from pass rate, so "degraded" has no direct entry in
// the test-status palette; it reads as the same amber as a broken test.
const STATUS_CLASS = {
  passed: STATUS_TEXT_CLASSES.passed,
  degraded: STATUS_TEXT_CLASSES.broken,
  failed: STATUS_TEXT_CLASSES.failed,
  skipped: STATUS_TEXT_CLASSES.skipped,
} as const

export function RunSuiteChip({ suite, runPath }: RunSuiteChipProps) {
  const label = suite.display_name || suite.slug
  const shardCount = suite.builds?.length ?? 1
  const isSharded = shardCount > 1
  // "Degraded" is derived from pass rate, so a suite at 95% was drawn as an
  // amber warning although tests in it failed. Any failure reads as ✗.
  const status = suite.failed > 0 ? 'failed' : suite.status

  const href = runPath ? `${runPath}#suite-${suite.project_id}` : suiteReportHref(suite)
  // The glyph is hidden and the counts are bare numbers, so the name says what they mean.
  // A passing suite needs no name: RunSuiteChips draws chips only for failing ones.
  const failedText = `${label}: ${suite.failed} failed${isSharded ? ` across ${shardCount} shards` : ''}`

  const chip = (
    <NavLink
      to={href}
      aria-label={suite.failed > 0 ? failedText : undefined}
      className="hover:bg-accent inline-flex max-w-full items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors"
      data-testid="run-suite-chip"
    >
      <span aria-hidden="true" className={STATUS_CLASS[status]}>
        {STATUS_ICON[status]}
      </span>
      <span className="truncate font-medium">{label}</span>
      {suite.failed > 0 && (
        // Ink on the red tint (5.6:1); red on its own tint is 3.75:1 by day.
        <Badge variant="failed" className="text-foreground px-1.5 py-0 text-[10px]">
          {suite.failed}
        </Badge>
      )}
      {isSharded && (
        <span
          className="text-fact inline-flex items-center gap-0.5"
          data-testid="run-suite-chip-shards"
        >
          <Split size={10} aria-hidden="true" />
          {shardCount}
        </span>
      )}
    </NavLink>
  )

  if (!isSharded) return chip

  return (
    <Tooltip>
      <TooltipTrigger asChild>{chip}</TooltipTrigger>
      {/* Below the chip: opening upward covered the run's own ID and branch. */}
      <TooltipContent side="bottom">{failedText}</TooltipContent>
    </Tooltip>
  )
}
