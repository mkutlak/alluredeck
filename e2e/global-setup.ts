import * as path from 'node:path'
import * as os from 'node:os'
import { AllureDeckClient } from './fixtures/alluredeck-client'

export default async function globalSetup() {
  console.log('\n[global-setup] Preparing test data for AllureDeck e2e tests...')

  const client = new AllureDeckClient()
  await client.login('admin', 'admin')
  console.log('  Authenticated as admin')

  const tmpDir = os.tmpdir()
  const fixturesDir = path.join(process.cwd(), 'fixtures')

  // Upload sample Playwright report FIRST (stored in latest/)
  const pwTarGz = path.join(tmpDir, 'e2e-playwright-report.tar.gz')
  AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-playwright-report'), pwTarGz)
  await client.uploadPlaywrightReport('e2e-demo', pwTarGz)

  // Upload sample Allure results (runner creates build + copies Playwright from latest/)
  const countBefore = await client.getReportCount('e2e-demo')
  const allureTarGz = path.join(tmpDir, 'e2e-allure-results.tar.gz')
  AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-results'), allureTarGz)
  await client.uploadAllureResults('e2e-demo', allureTarGz)

  // Wait for the NEW report to be generated
  await client.waitForNewReport('e2e-demo', countBefore)

  console.log('[global-setup] Test data ready.\n')
}
