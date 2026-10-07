import * as os from 'node:os'
import * as path from 'node:path'
import { test, expect } from '../fixtures/auth'
import { AllureDeckClient } from '../fixtures/alluredeck-client'
import { USERNAME, PASSWORD } from '../fixtures/credentials'

// A group with one suite whose build carries CI metadata, so it forms a pipeline run.
interface RunScenario {
  groupId: number
  suiteId: number
  pipelineId: string
}

async function setupRun(): Promise<RunScenario> {
  const client = new AllureDeckClient()
  await client.login(USERNAME, PASSWORD)

  const stamp = Date.now()
  const group = await client.createProject(`e2e-run-group-${stamp}`)
  const suite = await client.createProject(`e2e-run-suite-${stamp}`, group.project_id)
  const pipelineId = String(stamp)

  const tarGz = path.join(os.tmpdir(), `e2e-run-suite-${stamp}.tar.gz`)
  AllureDeckClient.tarGzDirectory(path.join(process.cwd(), 'fixtures', 'sample-results'), tarGz)
  const latestBefore = await client.getLatestReportId(String(suite.project_id))
  await client.uploadAllureResults(String(suite.project_id), tarGz, {
    ci_branch: 'feature/run-page-deep-link',
    ci_commit_sha: 'abcdef0123456789abcdef0123456789abcdef01',
    ci_pipeline_id: pipelineId,
  })
  await client.waitForNewReport(String(suite.project_id), latestBefore)

  return { groupId: group.project_id, suiteId: suite.project_id, pipelineId }
}

async function cleanupRun(scenario: RunScenario): Promise<void> {
  const client = new AllureDeckClient()
  await client.login(USERNAME, PASSWORD)
  await client.deleteProject(String(scenario.suiteId))
  await client.deleteProject(String(scenario.groupId))
}

test.describe('Run page', () => {
  let scenario: RunScenario

  test.beforeAll(async () => {
    scenario = await setupRun()
  })

  test.afterAll(async () => {
    if (scenario) await cleanupRun(scenario)
  })

  test('a #suite- deep link lands on the run with that suite shown and focused', async ({
    authenticatedPage: page,
  }) => {
    const { groupId, suiteId, pipelineId } = scenario
    await page.goto(`/runs/${groupId}/${pipelineId}#suite-${suiteId}`)

    await expect(page.getByTestId('run-page')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('heading', { level: 1, name: pipelineId })).toBeVisible()
    await expect(page.getByText('feature/run-page-deep-link')).toBeVisible()
    // The suite passed, so it sits in the collapsed list that the hash must open. The id
    // is the deep-link contract itself, so it is the locator here.
    const suiteRow = page.locator(`#suite-${suiteId}`)
    await expect(suiteRow).toBeVisible()
    await expect(suiteRow).toBeFocused()
    await expect(page).toHaveTitle(`${pipelineId} · feature/run-page-deep-link — AllureDeck`)
  })

  test('an unknown run shows "Run not found" with a way back to Runs', async ({
    authenticatedPage: page,
  }) => {
    await page.goto(`/runs/${scenario.groupId}/no-such-run`)
    await expect(page.getByRole('heading', { level: 1, name: 'Run not found' })).toBeVisible({
      timeout: 10_000,
    })
    await page.getByRole('link', { name: 'Back to Runs' }).click()
    await expect(page.getByTestId('runs-feed')).toBeVisible({ timeout: 10_000 })
  })
})
