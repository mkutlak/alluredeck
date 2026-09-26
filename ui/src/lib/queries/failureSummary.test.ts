import { describe, it, expect, vi } from 'vitest'
import { failureSummaryOptions } from './failureSummary'

vi.mock('@/api/failures', () => ({
  fetchFailureSummary: vi.fn(),
}))

describe('failureSummaryOptions', () => {
  // Fetch only when the LLM feature is on, the panel is open, and both ids exist.
  it.each<[string, Parameters<typeof failureSummaryOptions>, boolean]>([
    ['all conditions hold', [1, 42, 'h1', true, true], true],
    ['llmEnabled is false', [1, 42, 'h1', false, true], false],
    ['open is false', [1, 42, 'h1', true, false], false],
    ['buildId is 0', [1, 0, 'h1', true, true], false],
    ['historyId is empty', [1, 42, '', true, true], false],
  ])('%s -> enabled %s', (_, args, enabled) => {
    expect(failureSummaryOptions(...args).enabled).toBe(enabled)
  })
})
