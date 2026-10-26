import { defineConfig, devices } from '@playwright/test'

// Standalone config for the documentation screenshots; the default e2e config (testDir ./tests)
// never picks these up. Run via `mise run docs:screenshots`.
export default defineConfig({
  testDir: '.',
  testMatch: 'screenshots.spec.ts',
  outputDir: '../test-results/docs',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 120_000,
  reporter: [['list']],

  use: {
    ...devices['Desktop Chrome'],
    baseURL: process.env.ALLUREDECK_URL ?? 'http://localhost:7474',
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    colorScheme: 'light',
    timezoneId: 'UTC',
    locale: 'en-US',
    trace: 'off',
    video: 'off',
    screenshot: 'off',
  },
})
