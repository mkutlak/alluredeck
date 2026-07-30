import type { PipelineRun } from '@/types/api'

/**
 * The key the API groups a run by: its CI pipeline ID, or the commit SHA when
 * no pipeline ID was reported. Failures for a whole run are fetched with it.
 */
export function runKeyOf(run: PipelineRun): string {
  return run.pipeline_id || run.commit_sha
}
