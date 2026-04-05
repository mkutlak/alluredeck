import { test as base, type Page } from '@playwright/test'

export const test = base.extend<{ authenticatedPage: Page }>({
  authenticatedPage: async ({ page }, use) => {
    await page.goto('/login')
    await page.getByLabel('Username').fill('admin')
    await page.getByLabel('Password').fill('admin')
    await page.getByRole('button', { name: 'Sign in' }).click()
    await page.waitForURL('/')
    await use(page)
  },
})

export { expect } from '@playwright/test'
