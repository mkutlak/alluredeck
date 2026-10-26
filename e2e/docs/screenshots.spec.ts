/**
 * Captures the documentation screenshots against a stack seeded by `seed-demo.ts`.
 * Run via `mise run docs:screenshots`; output goes to $SCREENSHOT_DIR (default ../docs/screenshots).
 *
 * Every shot waits for real content (no skeletons, spinners or toasts) and resolves project,
 * run, report and test ids through the API, so nothing here is hard-coded to a database id.
 */
import * as path from 'node:path'
import { test, expect } from '../fixtures/auth'
import { AllureDeckClient } from '../fixtures/alluredeck-client'
import { USERNAME, PASSWORD } from '../fixtures/credentials'
import type { Locator, Page } from '@playwright/test'

const OUT = process.env.SCREENSHOT_DIR ?? '../docs/screenshots'

const PARENT = 'webshop'
const WEB = 'webshop-web'
const DEMO_USER = 'priya.raman@example.com'

interface Ids {
  groupId: number
  webId: number
  runKey: string
  latestReport: string
  greenReport: string
  historyId: string
  traceSource: string
  demoUserPassword: string
}

let ids: Ids

interface PipelineRun {
  pipeline_id: string
  suites: Array<{ slug: string; build_id: number; build_number: number; failed: number }>
}

test.beforeAll(async () => {
  const client = new AllureDeckClient()
  await client.login(USERNAME, PASSWORD)

  const projects = await client.listProjects()
  const group = projects.find((p) => p.slug === PARENT)
  const web = projects.find((p) => p.slug === WEB)
  if (!group || !web) throw new Error('Demo projects are missing; run docs/seed-demo.ts first')

  const runs = await client.api<{ data: PipelineRun[] }>(
    'GET',
    `/projects/${group.project_id}/pipeline-runs?per_page=50`,
  )
  const latestRun = runs.data[0]
  const latestWeb = latestRun.suites.find((s) => s.slug === WEB)
  if (!latestWeb || latestWeb.failed === 0) throw new Error('Latest webshop run is expected to be failing')
  // Most recent webshop-web build without failures: the baseline for the comparison shot.
  const green = runs.data.flatMap((r) => r.suites).find((s) => s.slug === WEB && s.failed === 0)
  if (!green) throw new Error('No fully green webshop-web build found')

  const failing = await client.api<{ data: Array<{ test_name: string; history_id: string }> }>(
    'GET',
    `/projects/${web.project_id}/builds/${latestWeb.build_id}/tests`,
  )
  const regression = failing.data.find((t) => t.test_name === 'guest can place an order')
  if (!regression) throw new Error('Regression test not found among failing tests')

  const latestReport = String(latestWeb.build_number)
  const attachments = await client.api<{
    data: { groups: Array<{ test_name: string; attachments: Array<{ name: string; source: string }> }> }
  }>('GET', `/projects/${web.project_id}/reports/${latestReport}/attachments`)
  const trace = attachments.data.groups
    .filter((g) => g.test_name === regression.test_name)
    .flatMap((g) => g.attachments)
    .find((a) => a.name === 'trace')
  if (!trace) throw new Error('Trace attachment not found')

  // The profile page looks empty for the env-configured admin, so it is captured as a demo user.
  const users = await client.api<{ data: { users: Array<{ id: number; email: string }> } }>('GET', '/users?limit=100')
  const demoUser = users.data.users.find((u) => u.email === DEMO_USER)
  if (!demoUser) throw new Error(`Demo user ${DEMO_USER} not found`)
  const reset = await client.api<{ data: { temp_password: string } }>('POST', `/users/${demoUser.id}/password`)

  ids = {
    groupId: group.project_id,
    webId: web.project_id,
    runKey: latestRun.pipeline_id,
    latestReport,
    greenReport: String(green.build_number),
    historyId: regression.history_id,
    traceSource: trace.source,
    demoUserPassword: reset.data.temp_password,
  }
})

/** Waits until nothing on the page looks like loading, then dismisses toasts and parks the mouse. */
async function settle(page: Page, extraWaitMs: number) {
  await page.waitForLoadState('networkidle')
  await expect(page.locator('.animate-pulse, .animate-spin, [aria-busy="true"]')).toHaveCount(0, {
    timeout: 15_000,
  })
  for (const close of await page.locator('[data-radix-toast-viewport] [toast-close]').all()) await close.click()
  await expect(page.locator('[data-radix-toast-viewport] [data-state="open"]')).toHaveCount(0)
  await page.mouse.move(1439, 899)
  // Recharts animates with JS, which `animations: 'disabled'` does not stop.
  await page.waitForTimeout(extraWaitMs)
}

async function shot(page: Page, name: string, opts: { target?: Locator; extraWaitMs?: number } = {}) {
  await settle(page, opts.extraWaitMs ?? 500)
  await (opts.target ?? page).screenshot({ path: path.join(OUT, name), animations: 'disabled' })
}

const CHART_WAIT = 2500

test.describe('documentation screenshots', () => {
  test('login.png', async ({ page }) => {
    await page.goto('/login')
    await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible()
    await shot(page, 'login.png')
  })

  test('runs-feed.png', async ({ authenticatedPage: page }) => {
    await page.goto('/')
    await expect(page.getByTestId('runs-feed')).toBeVisible()
    await expect(page.getByTestId('run-row').first()).toBeVisible()
    await shot(page, 'runs-feed.png')
  })

  test('run-page.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/runs/${ids.groupId}/${ids.runKey}`)
    await expect(page.getByTestId('run-page')).toBeVisible()
    await expect(page.getByTestId('run-failures')).toBeVisible()
    await shot(page, 'run-page.png')
  })

  test('pipeline-runs.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${PARENT}`)
    await expect(page.getByText(`Pipeline ${ids.runKey}`)).toBeVisible()
    await page.getByRole('button', { name: new RegExp(`^Pipeline ${ids.runKey}\\b`) }).click({ position: { x: 700, y: 20 } })
    await expect(page.getByText('webshop-mobile').first()).toBeVisible()
    await shot(page, 'pipeline-runs.png')
  })

  test('dashboard.png', async ({ authenticatedPage: page }) => {
    await page.goto('/projects')
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
    await page.getByRole('button', { name: `Expand ${PARENT}` }).click()
    await expect(page.getByText('webshop-mobile')).toBeVisible()
    await shot(page, 'dashboard.png')
  })

  test('project-actions-menu.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}`)
    await expect(page.getByTestId('report-row').first()).toBeVisible()
    await page.getByRole('button', { name: 'Project actions' }).click()
    await expect(page.getByRole('menuitem', { name: 'Send results' })).toBeVisible()
    await shot(page, 'project-actions-menu.png')
  })

  test('project-overview.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}`)
    await expect(page.getByTestId('report-row').first()).toBeVisible()
    await shot(page, 'project-overview.png')
  })

  test('report-history.png', async ({ authenticatedPage: page }) => {
    // Tall enough that the whole table is on screen; the element is clipped to the viewport otherwise.
    await page.setViewportSize({ width: 1440, height: 1500 })
    await page.goto(`/projects/${WEB}`)
    await expect(page.getByTestId('report-row')).toHaveCount(12)
    await shot(page, 'report-history.png', { target: page.getByTestId('report-list') })
  })

  test('analytics-trends.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/analytics`)
    await expect(page.locator('.recharts-surface').first()).toBeVisible()
    await shot(page, 'analytics-trends.png', { extraWaitMs: CHART_WAIT })
  })

  test('analytics-details.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/analytics`)
    const flaky = page
      .getByText('Flaky Impact', { exact: true })
      .locator('xpath=ancestor::*[contains(@class, "rounded")][1]')
    await expect(flaky).toBeVisible()
    await expect(flaky).not.toContainText('No flaky tests')
    // Bottom-align the Flaky Impact card so the Quality section (low performers + flaky) fills the frame.
    await flaky.evaluate((el) => el.scrollIntoView({ block: 'end' }))
    await shot(page, 'analytics-details.png', { extraWaitMs: CHART_WAIT })
  })

  test('known-issues.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/known-issues`)
    await expect(page.getByText('autocompletes product names')).toBeVisible()
    await shot(page, 'known-issues.png')
  })

  test('defects-list.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/defects`)
    await expect(page.getByText('order-confirmation').first()).toBeVisible()
    await shot(page, 'defects-list.png')
  })

  test('defects-detail.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/defects`)
    await page.getByText('order-confirmation').first().click()
    await expect(page.getByTestId('defect-detail')).toBeVisible()
    await expect(page.getByTestId('defect-tests-list')).toBeVisible()
    await shot(page, 'defects-detail.png')
  })

  test('timeline.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/timeline`)
    await expect(page.getByTestId('gantt-chart')).toBeVisible()
    await shot(page, 'timeline.png', { extraWaitMs: 1000 })
  })

  test('attachments.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/attachments`)
    // "latest" is ambiguous once several branches each have a latest build; pick the newest explicitly.
    await page.getByRole('combobox').nth(1).click()
    await page.getByRole('option', { name: new RegExp(`^#${ids.latestReport}\\b`) }).click()
    await expect(page.getByText(`Report #${ids.latestReport}`)).toBeVisible()
    await expect(page.getByText('guest can place an order')).toBeVisible()
    await shot(page, 'attachments.png')
  })

  test('test-history.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/tests?history_id=${ids.historyId}&branch=main`)
    await expect(page.getByText(/\d+ builds shown/)).toBeVisible()
    await expect(page.getByRole('row')).toHaveCount(10)
    await shot(page, 'test-history.png')
  })

  test('build-comparison.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/compare?a=${ids.greenReport}&b=${ids.latestReport}`)
    await expect(page.getByText('guest can place an order')).toBeVisible()
    await shot(page, 'build-comparison.png')
  })

  test('report-viewer.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/reports/${ids.latestReport}`)
    const frame = page.frameLocator('[data-testid="allure-iframe"]')
    await expect(frame.locator('body')).toContainText('guest can place an order', { timeout: 30_000 })
    await shot(page, 'report-viewer.png', { extraWaitMs: 1500 })
  })

  test('trace-viewer.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}/trace/${ids.traceSource}?reportId=${ids.latestReport}`)
    const frame = page.frameLocator('iframe')
    await expect(frame.locator('.action-title').first()).toBeVisible({ timeout: 30_000 })
    await shot(page, 'trace-viewer.png', { extraWaitMs: 2000 })
  })

  test('send-results.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/projects/${WEB}`)
    await expect(page.getByTestId('report-row').first()).toBeVisible()
    await page.getByRole('button', { name: 'Project actions' }).click()
    await page.getByRole('menuitem', { name: 'Send results' }).click()
    await expect(page.getByRole('dialog').getByRole('heading', { name: 'Send results' })).toBeVisible()
    await shot(page, 'send-results.png')
  })

  test('search.png', async ({ authenticatedPage: page }) => {
    await page.goto('/')
    await expect(page.getByTestId('runs-feed')).toBeVisible()
    await page.keyboard.press('Control+k')
    const dialog = page.getByRole('dialog')
    await dialog.locator('input').fill('checkout')
    await expect(dialog.locator('[cmdk-item]').nth(2)).toBeVisible()
    await shot(page, 'search.png')
  })

  test('admin-monitor.png', async ({ authenticatedPage: page }) => {
    await page.goto('/admin')
    await expect(page.getByRole('heading', { name: 'System Monitor' })).toBeVisible()
    await expect(page.getByText('completed').first()).toBeVisible()
    await shot(page, 'admin-monitor.png')
  })

  test('user-management.png', async ({ authenticatedPage: page }) => {
    await page.goto('/settings/users')
    await expect(page.getByText(DEMO_USER)).toBeVisible()
    await shot(page, 'user-management.png')
  })

  test('user-profile.png', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel('Username').fill(DEMO_USER)
    await page.getByLabel('Password').fill(ids.demoUserPassword)
    await page.getByRole('button', { name: 'Sign in' }).click()
    await page.waitForURL('/')
    await page.goto('/settings/profile')
    await expect(page.getByText(DEMO_USER)).toBeVisible()
    await shot(page, 'user-profile.png')
  })

  test('webhooks-list.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/settings/webhooks?project=${ids.webId}`)
    await expect(page.getByText('Slack #webshop-ci')).toBeVisible()
    await expect(page.getByRole('button', { name: 'webshop/webshop-web' })).toBeVisible()
    await shot(page, 'webhooks-list.png')
  })

  test('webhook-edit.png', async ({ authenticatedPage: page }) => {
    await page.goto(`/settings/webhooks?project=${ids.webId}`)
    await page.getByRole('button', { name: 'Edit webhook Slack #webshop-ci' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByRole('dialog').locator('input').first()).toHaveValue('Slack #webshop-ci')
    await page.getByRole('dialog').getByRole('heading').click() // drop the autofocus text selection
    await shot(page, 'webhook-edit.png')
  })

  test('api-keys.png', async ({ authenticatedPage: page }) => {
    await page.goto('/settings/api-keys')
    await expect(page.getByText('GitLab CI uploader')).toBeVisible()
    await shot(page, 'api-keys.png')
  })
})
