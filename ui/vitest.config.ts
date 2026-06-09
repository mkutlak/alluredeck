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
        // Regression floor pinned just below current coverage so it can be
        // ENFORCED in CI (previously these thresholds were configured but the
        // CI step ran `vitest run` without --coverage, so they never applied
        // and actual coverage drifted to ~61-67%). Ratchet these upward toward
        // the 80/80/70/80 target as coverage improves; never lower them.
        thresholds: {
          lines: 65,
          functions: 60,
          branches: 62,
          statements: 65,
        },
      },
    },
  }),
)
