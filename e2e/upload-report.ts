import * as path from 'node:path'
import * as fs from 'node:fs'
import * as os from 'node:os'
import { AllureDeckClient } from './fixtures/alluredeck-client'

async function main() {
  const cwd = process.cwd()
  const reportDir = path.join(cwd, 'playwright-report')
  const allureDir = path.join(cwd, 'allure-results')

  if (!fs.existsSync(path.join(reportDir, 'index.html'))) {
    console.log('[upload] No Playwright HTML report found — skipping.')
    process.exit(0)
  }

  console.log('[upload] Uploading report to AllureDeck...')

  const client = new AllureDeckClient()
  await client.login('admin', 'admin')

  // 1. Upload Allure results FIRST — creates the build
  if (fs.existsSync(allureDir)) {
    const latestBefore = await client.getLatestReportId('e2e-self-test')
    const allureTarGz = path.join(os.tmpdir(), 'e2e-self-test-allure.tar.gz')
    AllureDeckClient.tarGzDirectoryFiltered(allureDir, allureTarGz, ['*.json'])
    await client.uploadAllureResults('e2e-self-test', allureTarGz)
    await client.waitForNewReport('e2e-self-test', latestBefore)
  } else {
    console.log('[upload] No allure-results found — skipping Allure upload.')
  }

  // 2. Upload Playwright HTML report directly to the build — no polling needed.
  const buildNumber = await client.getLatestReportId('e2e-self-test') ?? 'latest'
  const pwTarGz = path.join(os.tmpdir(), 'e2e-self-test-report.tar.gz')
  AllureDeckClient.tarGzDirectory(reportDir, pwTarGz)
  await client.uploadPlaywrightReport('e2e-self-test', pwTarGz, buildNumber)

  console.log(`[upload] Report uploaded successfully!`)
  console.log(`[upload] View at: ${client.getReportUrl('e2e-self-test', buildNumber)}`)
}

main().catch((err) => {
  console.error('[upload] Failed:', err.message)
  process.exit(1)
})
