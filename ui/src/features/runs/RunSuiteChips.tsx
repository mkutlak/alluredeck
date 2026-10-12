import { useState } from 'react'

import { RunSuiteChip } from './RunSuiteChip'
import type { PipelineSuite } from '@/types/api'

export interface RunSuiteChipsProps {
  suites: PipelineSuite[]
  /** The run's page, so each chip opens its suite's section there. */
  runPath?: string
  /** Where the failing tests came from ("2 new", "3 inherited"), after the passed count. */
  origins?: string[]
}

/** Chips shown before the "+N failing" expander kicks in. */
const VISIBLE_LIMIT = 4

/**
 * Renders only the suites that failed, worst first.
 *
 * A run can hold twenty suites of which most passed. Passing suites are not
 * news and cost three wrapped lines of chips per run, so they collapse to a
 * count and the failing ones lead.
 */
export function RunSuiteChips({ suites, runPath, origins = [] }: RunSuiteChipsProps) {
  const [showAll, setShowAll] = useState(false)

  const failing = suites
    .filter((s) => s.failed > 0)
    .sort((a, b) => b.failed - a.failed || a.slug.localeCompare(b.slug))
  const passedCount = suites.length - failing.length

  if (failing.length === 0) return null

  const visible = showAll ? failing : failing.slice(0, VISIBLE_LIMIT)
  const hiddenCount = failing.length - visible.length

  return (
    <div className="flex flex-wrap items-center gap-1.5" data-testid="run-suite-chips">
      {visible.map((suite) => (
        <RunSuiteChip key={suite.project_id} suite={suite} runPath={runPath} />
      ))}

      {hiddenCount > 0 && (
        <button
          type="button"
          onClick={() => setShowAll(true)}
          className="text-fact hover:text-foreground text-xs underline-offset-2 hover:underline"
          data-testid="run-suite-chips-more"
        >
          {`+${hiddenCount} failing`}
        </button>
      )}

      {passedCount > 0 && <span className="text-fact text-xs">{`· ${passedCount} passed`}</span>}

      {/* contents: each part is a flex item, spaced like "· N passed" and wrapping on its own. */}
      {origins.length > 0 && (
        <span className="contents" data-testid="run-suite-chips-origins">
          <span className="sr-only">Failing tests:</span>
          {origins.map((part) => (
            <span key={part} className="text-fact text-xs">{`· ${part}`}</span>
          ))}
        </span>
      )}
    </div>
  )
}
