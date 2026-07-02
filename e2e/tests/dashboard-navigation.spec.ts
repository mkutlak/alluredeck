import { test, expect } from '../fixtures/project'

test.describe('Dashboard & Navigation', () => {
  test('dashboard shows project cards', async ({ authenticatedPage: page, freshProject }) => {
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({
      timeout: 10_000,
    })
    // Switch to "All" view to see individual projects (default is "Grouped")
    await page.getByRole('main').getByRole('button', { name: 'All' }).click()
    await expect(
      page.getByRole('main').getByRole('link', { name: freshProject.projectSlug }),
    ).toBeVisible({ timeout: 10_000 })
  })

  test('navigate into project overview', async ({ authenticatedPage: page, freshProject }) => {
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({
      timeout: 10_000,
    })
    await page.getByRole('main').getByRole('button', { name: 'All' }).click()
    await expect(
      page.getByRole('main').getByRole('link', { name: freshProject.projectSlug }),
    ).toBeVisible({ timeout: 10_000 })

    await page.getByRole('main').getByRole('link', { name: freshProject.projectSlug }).click()
    await expect(page).toHaveURL(/\/projects\/\d+/, { timeout: 10_000 })

    await expect(page.getByTestId('sidebar-nav-overview')).toBeVisible()
  })

  test('navigate project tabs', async ({ authenticatedPage: page, freshProject }) => {
    // Post-IA-redesign: project sub-navigation is a horizontal tab strip on the
    // project page (not the sidebar, which no longer carries project nav).
    // The tabs kept the same sidebar-nav-* testids to minimize e2e churn.
    // Tabs navigate by numeric project_id (the repo URL rule), so a slug URL
    // canonicalizes to /projects/<id>/... on the first tab click — assert the
    // route suffix rather than the identifier form.
    await page.goto(`/projects/${freshProject.projectSlug}`)
    const tabs = page.getByRole('navigation', { name: 'Project sections' })
    await expect(tabs.getByTestId('sidebar-nav-overview')).toBeVisible({ timeout: 10_000 })

    // Analytics
    await tabs.getByTestId('sidebar-nav-analytics').click()
    await expect(page).toHaveURL(/\/projects\/[^/]+\/analytics$/)

    // Defects
    await tabs.getByTestId('sidebar-nav-defects').click()
    await expect(page).toHaveURL(/\/projects\/[^/]+\/defects$/)

    // Timeline
    await tabs.getByTestId('sidebar-nav-timeline').click()
    await expect(page).toHaveURL(/\/projects\/[^/]+\/timeline$/)
  })

  test('view allure report in iframe', async ({ authenticatedPage: page, freshProject }) => {
    await page.goto(`/projects/${freshProject.projectSlug}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })

    const reportRow = page.getByTestId('report-row').first()
    await expect(reportRow).toBeVisible({ timeout: 10_000 })
    await reportRow.getByRole('link', { name: 'View' }).click()
    await page.waitForURL(/\/reports\//, { timeout: 10_000 })

    await expect(page.getByTestId('allure-iframe')).toBeVisible()
    await expect(page.getByRole('link', { name: /open in new tab/i })).toBeVisible()
  })

  test('toggle playwright/allure view', async ({ authenticatedPage: page, freshProject }) => {
    await page.goto(`/projects/${freshProject.projectSlug}`)
    await expect(page.getByTestId('project-overview')).toBeVisible({ timeout: 10_000 })

    const reportRow = page.getByTestId('report-row').first()
    await expect(reportRow).toBeVisible({ timeout: 10_000 })
    await reportRow.getByRole('link', { name: 'View' }).click()
    await page.waitForURL(/\/reports\//, { timeout: 10_000 })

    const playwrightToggle = page.getByTestId('view-toggle-playwright')
    const allureToggle = page.getByTestId('view-toggle-allure')

    if (await playwrightToggle.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await allureToggle.click()
      const iframe = page.getByTestId('allure-iframe')
      await expect(iframe).toHaveAttribute('src', /\/reports\//)

      await playwrightToggle.click()
      await expect(iframe).toHaveAttribute('src', /\/playwright-reports\//)
    }
  })

  test('custom attachments', async ({ authenticatedPage: page }, testInfo) => {
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({
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
    const bodyHtml = await page
      .locator('body')
      .innerHTML()
      .catch(() => '<p>N/A</p>')
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
