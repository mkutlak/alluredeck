import { test, expect } from '../fixtures/auth'

test.describe('Dashboard & Navigation', () => {
  test('dashboard shows project cards', async ({ authenticatedPage: page }) => {
    await expect(page.getByRole('heading', { name: 'Projects Dashboard' })).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByRole('main').getByRole('link', { name: 'e2e-demo' })).toBeVisible({
      timeout: 10_000,
    })
  })

  test('navigate into project overview', async ({ authenticatedPage: page }) => {
    await expect(page.getByRole('main').getByRole('link', { name: 'e2e-demo' })).toBeVisible({
      timeout: 10_000,
    })

    await page.getByRole('main').getByRole('link', { name: 'e2e-demo' }).first().click()
    await page.waitForURL(/\/projects\/e2e-demo/, { timeout: 10_000 })

    await expect(page.getByRole('link', { name: 'Overview' })).toBeVisible()
  })

  test('navigate project tabs', async ({ authenticatedPage: page }) => {
    await page.goto('/projects/e2e-demo')
    await expect(page.getByText('Overview')).toBeVisible({ timeout: 10_000 })

    // Analytics
    await page.getByRole('link', { name: 'Analytics' }).click()
    await expect(page).toHaveURL(/\/projects\/e2e-demo\/analytics/)

    // Defects
    await page.getByRole('link', { name: 'Defects' }).click()
    await expect(page).toHaveURL(/\/projects\/e2e-demo\/defects/)

    // Timeline
    await page.getByRole('link', { name: 'Timeline' }).click()
    await expect(page).toHaveURL(/\/projects\/e2e-demo\/timeline/)
  })

  test('view allure report in iframe', async ({ authenticatedPage: page }) => {
    await page.goto('/projects/e2e-demo')
    await expect(page.getByText('Overview')).toBeVisible({ timeout: 10_000 })

    const reportLink = page.getByRole('link', { name: /report|#1/i }).first()
    if (await reportLink.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await reportLink.click()
      await page.waitForURL(/\/reports\//)

      const iframe = page.locator('iframe')
      await expect(iframe).toBeVisible()
      await expect(page.getByRole('link', { name: /open in new tab/i })).toBeVisible()
    }
  })

  test('toggle playwright/allure view', async ({ authenticatedPage: page }) => {
    await page.goto('/projects/e2e-demo')
    await expect(page.getByText('Overview')).toBeVisible({ timeout: 10_000 })

    const reportLink = page.getByRole('link', { name: /report|#1/i }).first()
    if (await reportLink.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await reportLink.click()
      await page.waitForURL(/\/reports\//)

      const playwrightToggle = page.getByRole('button', { name: 'Playwright' })
      const allureToggle = page.getByRole('button', { name: 'Allure' })

      if (await playwrightToggle.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await allureToggle.click()
        const iframe = page.locator('iframe')
        await expect(iframe).toHaveAttribute('src', /\/reports\//)

        await playwrightToggle.click()
        await expect(iframe).toHaveAttribute('src', /\/playwright-reports\//)
      }
    }
  })

  test('custom attachments', async ({ authenticatedPage: page }, testInfo) => {
    await expect(page.getByRole('heading', { name: 'Projects Dashboard' })).toBeVisible({
      timeout: 10_000,
    })

    // Attach a JSON summary
    const summary = {
      timestamp: new Date().toISOString(),
      url: page.url(),
      title: await page.title(),
    }
    await testInfo.attach('page-summary.json', {
      body: JSON.stringify(summary, null, 2),
      contentType: 'application/json',
    })

    // Attach an HTML snippet
    const bodyHtml = await page.locator('body').innerHTML().catch(() => '<p>N/A</p>')
    await testInfo.attach('dashboard-snapshot.html', {
      body: `<!DOCTYPE html><html><head><title>Dashboard Snapshot</title></head><body>${bodyHtml}</body></html>`,
      contentType: 'text/html',
    })

    // Attach a manual full-page screenshot
    const screenshot = await page.screenshot({ fullPage: true })
    await testInfo.attach('full-dashboard-screenshot.png', {
      body: screenshot,
      contentType: 'image/png',
    })

    await expect(page).toHaveTitle(/AllureDeck/)
  })
})
