import * as path from 'node:path'
import * as os from 'node:os'
import { test as authTest } from './auth'
import { AllureDeckClient } from './alluredeck-client'

interface FreshProject {
  projectSlug: string
}

export const test = authTest.extend<{ freshProject: FreshProject }>({
  freshProject: async ({}, use, testInfo) => {
    const slug =
      'e2e-' +
      testInfo.title
        .toLowerCase()
        .replace(/\W+/g, '-')
        .replace(/^-+|-+$/g, '') +
      '-' +
      Date.now()

    const client = new AllureDeckClient()
    await client.login('admin', 'admin')

    const fixturesDir = path.join(process.cwd(), 'fixtures')
    const tmpDir = os.tmpdir()

    // Upload Playwright report first (stored in latest/)
    const pwTarGz = path.join(tmpDir, `${slug}-playwright.tar.gz`)
    AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-playwright-report'), pwTarGz)
    await client.uploadPlaywrightReport(slug, pwTarGz)

    // Upload Allure results (runner creates build + copies Playwright from latest/)
    const latestBefore = await client.getLatestReportId(slug)
    const allureTarGz = path.join(tmpDir, `${slug}-allure.tar.gz`)
    AllureDeckClient.tarGzDirectory(path.join(fixturesDir, 'sample-results'), allureTarGz)
    await client.uploadAllureResults(slug, allureTarGz)

    // Wait for the report to be generated
    await client.waitForNewReport(slug, latestBefore)

    await use({ projectSlug: slug })

    // Teardown: delete the project
    await client.deleteProject(slug)
  },
})

export { expect } from '@playwright/test'
