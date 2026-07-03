import { test, expect } from '../fixtures/project'

test.describe('Runs Feed', () => {
  test('"/" shows the Runs heading and runs-feed root', async ({ authenticatedPage: page }) => {
    await expect(page.getByRole('heading', { name: 'Runs' })).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('runs-feed')).toBeVisible({ timeout: 10_000 })
  })

  test('shows empty state with a link to /projects when no builds carry CI metadata', async ({
    authenticatedPage: page,
    freshProject,
  }) => {
    // freshProject uploads Allure + Playwright results without CI metadata (no branch/commit),
    // so the global runs feed treats it as absent and falls back to its empty state.
    await page.goto('/')
    await expect(page.getByTestId('runs-feed')).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByText('Runs appear here when suites upload results with CI metadata.'),
    ).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('link', { name: 'Browse projects' })).toHaveAttribute(
      'href',
      '/projects',
    )
  })

  test('sidebar navigation round-trip: Runs → Projects → Runs', async ({
    authenticatedPage: page,
  }) => {
    const runsLink = page.getByRole('link', { name: 'Runs', exact: true })
    const projectsLink = page.getByRole('link', { name: 'Projects', exact: true })

    await expect(page).toHaveURL(/\/$/)
    await expect(runsLink).toHaveAttribute('aria-current', 'page')

    await projectsLink.click()
    await expect(page).toHaveURL(/\/projects$/)
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({ timeout: 10_000 })
    await expect(projectsLink).toHaveAttribute('aria-current', 'page')

    await runsLink.click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole('heading', { name: 'Runs' })).toBeVisible({ timeout: 10_000 })
    await expect(runsLink).toHaveAttribute('aria-current', 'page')
  })
})
