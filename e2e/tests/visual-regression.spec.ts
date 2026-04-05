import { test, expect } from '../fixtures/auth'

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

  test('dashboard page', async ({ authenticatedPage: page }) => {
    await expect(page.getByRole('heading', { name: 'Projects Dashboard' })).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByRole('main').getByRole('link', { name: 'e2e-demo' })).toBeVisible({
      timeout: 10_000,
    })

    await expect(page).toHaveScreenshot('dashboard.png', {
      maxDiffPixelRatio: 0.05,
    })
  })

  test('project overview', async ({ authenticatedPage: page }) => {
    await page.goto('/projects/e2e-demo')
    await expect(page.getByText('Overview')).toBeVisible({ timeout: 10_000 })

    await expect(page).toHaveScreenshot('project-overview.png', {
      maxDiffPixelRatio: 0.05,
    })
  })

  test('analytics page', async ({ authenticatedPage: page }) => {
    await page.goto('/projects/e2e-demo/analytics')
    await page.waitForLoadState('networkidle')

    await expect(page).toHaveScreenshot('analytics.png', {
      maxDiffPixelRatio: 0.05,
    })
  })

  test('report viewer', async ({ authenticatedPage: page }) => {
    await page.goto('/projects/e2e-demo')
    await expect(page.getByText('Overview')).toBeVisible({ timeout: 10_000 })

    const reportLink = page.getByRole('link', { name: /report|#1/i }).first()
    if (await reportLink.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await reportLink.click()
      await page.waitForURL(/\/reports\//)

      const iframe = page.locator('iframe')
      await expect(iframe).toBeVisible()

      await expect(page).toHaveScreenshot('report-viewer.png', {
        maxDiffPixelRatio: 0.05,
      })
    }
  })
})
