import { defineConfig, devices } from '@playwright/test'

const BASE_URL = process.env.ALLUREDECK_URL ?? 'http://localhost:7474'

export default defineConfig({
  testDir: './tests',
  outputDir: './test-results',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : 1,
  timeout: 30_000,

  globalSetup: './global-setup.ts',

  reporter: [
    ['html', { open: 'never' }],
    ['allure-playwright', { outputFolder: './allure-results' }],
    ['list'],
  ],

  use: {
    baseURL: BASE_URL,
    video: 'on',
    trace: 'on',
    screenshot: 'on',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
