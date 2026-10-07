import type { PipelineRun, PipelineSuite } from '@/types/api'

/**
 * The key the API groups a run by: its CI pipeline ID, or the commit SHA when
 * no pipeline ID was reported. Failures for a whole run are fetched with it.
 */
export function runKeyOf(run: PipelineRun): string {
  return run.pipeline_id || run.commit_sha
}

/** How a run is named on screen: its pipeline ID, else the short SHA. */
export function runIdOf(run: PipelineRun): string {
  return run.pipeline_id || run.commit_sha.slice(0, 7)
}

/** The run's own page. A run is (group, key); without a group there is no page. */
export function runPath(run: PipelineRun): string | undefined {
  return run.group_project_id != null
    ? `/runs/${run.group_project_id}/${encodeURIComponent(runKeyOf(run))}`
    : undefined
}

/**
 * A suite's Allure report. A sharded suite has no single report (several builds
 * contributed to it), so it goes to the project, where every build is listed.
 */
export function suiteReportHref(suite: PipelineSuite): string {
  return (suite.builds?.length ?? 1) > 1
    ? `/projects/${suite.project_id}`
    : `/projects/${suite.project_id}/reports/${suite.build_number}`
}
