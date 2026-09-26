import { defineConfig, mergeConfig } from 'vitest/config'
import { fileURLToPath, URL } from 'node:url'
import viteConfig from './vite.config'

export default mergeConfig(
  viteConfig,
  defineConfig({
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    test: {
      globals: true,
      environment: 'jsdom',
      setupFiles: ['./src/test/setup.ts', 'allure-vitest/setup'],
      pool: 'threads',
      reporters: ['default', 'allure-vitest/reporter'],
      coverage: {
        provider: 'v8',
        reporter: ['text', 'lcov', 'html'],
        include: [
          'src/features/**',
          'src/hooks/**',
          'src/lib/**',
          'src/store/**',
          'src/api/**',
        ],
        // Regression floor pinned at floor(actual) - 1 so it is ENFORCED in CI.
        // Re-baselined after the 2026-09 test prune (actual 68.7/63.1/66.2/67.8
        // lines/functions/branches/statements). Ratchet upward as coverage
        // improves; never lower them.
        thresholds: {
          lines: 67,
          functions: 62,
          branches: 65,
          statements: 66,
        },
      },
    },
  }),
)
