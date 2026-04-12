import { test, expect } from '../fixtures/project'

test.describe('Visual Regression', () => {
  test('login page', async ({ browser }) => {
    const context = await browser.newContext()
    const page = await context.newPage()

    await page.goto('/login')
    await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible({ timeout: 10_000 })

    await expect(page).toHaveScreenshot('login-page.png', {
      maxDiffPixelRatio: 0.05,
    })

    await context.close()
  })

  test('dashboard page', async ({ authenticatedPage: page, freshProject }) => {
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({
      timeout: 10_000,
    })
    await page.getByRole('main').getByRole('button', { name: 'All' }).click()
    await expect(
      page.getByRole('main').getByRole('link', { name: freshProject.projectSlug }),
    ).toBeVisible({ timeout: 10_000 })

    await expect(page).toHaveScreenshot('dashboard.png', {
      maxDiffPixelRatio: 0.05,
    })
  })

  test('project overview', async ({ authenticatedPage: page, freshProject }) => {
    await page.goto(`/projects/${freshProject.projectSlug}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })

    await expect(page).toHaveScreenshot('project-overview.png', {
      maxDiffPixelRatio: 0.05,
    })
  })

  test('analytics page', async ({ authenticatedPage: page, freshProject }) => {
    await page.goto(`/projects/${freshProject.projectSlug}/analytics`)
    await page.waitForLoadState('networkidle')

    await expect(page).toHaveScreenshot('analytics.png', {
      maxDiffPixelRatio: 0.05,
    })
  })

  test('report viewer', async ({ authenticatedPage: page, freshProject }) => {
    await page.goto(`/projects/${freshProject.projectSlug}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })

    const reportRow = page.getByTestId('report-row').first()
    await expect(reportRow).toBeVisible({ timeout: 10_000 })
    await reportRow.getByRole('link', { name: 'View' }).click()
    await page.waitForURL(/\/reports\//, { timeout: 10_000 })

    await expect(page.getByTestId('allure-iframe')).toBeVisible()

    await expect(page).toHaveScreenshot('report-viewer.png', {
      maxDiffPixelRatio: 0.05,
    })
  })
})
