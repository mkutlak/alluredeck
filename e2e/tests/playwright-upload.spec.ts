import { test, expect } from '@playwright/test'
import * as fs from 'node:fs'
import * as path from 'node:path'
import * as os from 'node:os'
import { AllureDeckClient } from '../fixtures/alluredeck-client'

const API_URL = process.env.ALLUREDECK_API_URL ?? 'http://localhost:5050/api/v1'

test.describe('Playwright Upload API', () => {
  let client: AllureDeckClient

  test.beforeAll(async () => {
    client = new AllureDeckClient()
    await client.login('admin', 'admin')
  })

  test('standalone Playwright upload with CI metadata creates a build', async () => {
    const projectId = 'e2e-pw-standalone'
    const fixturesDir = path.join(process.cwd(), 'fixtures')
    const tarGz = path.join(os.tmpdir(), 'e2e-pw-standalone.tar.gz')
    AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-playwright-report'), tarGz)

    // Upload Playwright report with CI metadata — should return 202 with job_id.
    const body = fs.readFileSync(tarGz)
    const url =
      `${API_URL}/projects/${projectId}/playwright` +
      `?force_project_creation=true` +
      `&execution_name=E2E+CI` +
      `&ci_branch=main` +
      `&ci_commit_sha=abc123`

    const res = await fetch(url, {
      method: 'POST',
      headers: {
        ...authHeaders(client),
        'Content-Type': 'application/gzip',
      },
      body,
    })

    const json = (await res.json()) as { data?: { job_id?: string }; metadata?: { message?: string } }
    console.log(`  Standalone upload response: ${res.status}`, JSON.stringify(json))
    expect(res.status).toBe(202)
    expect(json.data?.job_id).toBeTruthy()
    expect(json.metadata?.message).toBe('Playwright ingestion queued')
  })

  test('plain Playwright upload without CI metadata returns 200', async () => {
    const projectId = 'e2e-pw-plain'
    const fixturesDir = path.join(process.cwd(), 'fixtures')
    const tarGz = path.join(os.tmpdir(), 'e2e-pw-plain.tar.gz')
    AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-playwright-report'), tarGz)

    const body = fs.readFileSync(tarGz)
    const url = `${API_URL}/projects/${projectId}/playwright?force_project_creation=true`

    const res = await fetch(url, {
      method: 'POST',
      headers: {
        ...authHeaders(client),
        'Content-Type': 'application/gzip',
      },
      body,
    })

    const json = (await res.json()) as { data?: { status?: string } }
    expect(res.status).toBe(200)
    expect(json.data?.status).toBe('uploaded')
  })

  test('upload Playwright report with build_number writes directly', async () => {
    const projectId = 'e2e-pw-direct'
    const fixturesDir = path.join(process.cwd(), 'fixtures')

    // First create a build via Allure upload.
    const allureTarGz = path.join(os.tmpdir(), 'e2e-pw-direct-allure.tar.gz')
    AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-results'), allureTarGz)
    await client.uploadAllureResults(projectId, allureTarGz)
    await client.waitForNewReport(projectId, 0)
    const buildNumber = await client.getLatestReportId(projectId)

    // Upload Playwright report with build_number — should return 200.
    const pwTarGz = path.join(os.tmpdir(), 'e2e-pw-direct-report.tar.gz')
    AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-playwright-report'), pwTarGz)
    await client.uploadPlaywrightReport(projectId, pwTarGz, buildNumber)

    // Verify the Playwright report is immediately accessible at the build path.
    const found = await client.waitForPlaywrightReport(projectId, buildNumber, 5_000)
    expect(found).toBe(true)
  })
})

/** Extract auth headers from a logged-in client (reuse the token). */
function authHeaders(c: AllureDeckClient): Record<string, string> {
  // Access the private token via bracket notation for test convenience.
  const token = (c as unknown as { token: string }).token
  return { Authorization: `Bearer ${token}` }
}
