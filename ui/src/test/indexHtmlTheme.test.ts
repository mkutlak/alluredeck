import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import indexHtml from '../../index.html?raw'

// index.html applies the saved theme before first paint, because next-themes' own init script
// never runs in this client-only SPA. These tests execute that inline script against jsdom.
function loadThemeScript(): () => void {
  const body = [...indexHtml.matchAll(/<script>([\s\S]*?)<\/script>/g)]
    .map((m) => m[1] ?? '')
    .find((src) => src.includes("localStorage.getItem('theme')"))
  expect(body, 'index.html has no inline <script> reading localStorage "theme"').toBeDefined()
  // eslint-disable-next-line @typescript-eslint/no-implied-eval -- executing the page's own inline script
  return new Function(body ?? '') as () => void
}

function stubSystemTheme(prefersDark: boolean) {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: prefersDark && query === '(prefers-color-scheme: dark)',
  }))
}

const root = document.documentElement

describe('index.html theme bootstrap script', () => {
  // The fix only works if the script runs before first paint: in <head>, ahead of the app bundle.
  it('sits in <head>, before the app bundle', () => {
    const at = indexHtml.indexOf("localStorage.getItem('theme')")
    expect(at).toBeGreaterThan(-1)
    expect(at).toBeLessThan(indexHtml.indexOf('</head>'))
    expect(at).toBeLessThan(indexHtml.indexOf('<script type="module"'))
  })

  beforeEach(() => {
    localStorage.clear()
    root.className = ''
    root.style.colorScheme = ''
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it.each([
    ['dark', 'dark'],
    ['light', 'light'],
  ])('stored %s applies the %s theme', (stored, expected) => {
    localStorage.setItem('theme', stored)
    loadThemeScript()()
    expect(root.classList.contains(expected)).toBe(true)
    expect(root.classList).toHaveLength(1)
    expect(root.style.colorScheme).toBe(expected)
  })

  it.each([
    ['system', true, 'dark'],
    ['system', false, 'light'],
    [null, true, 'dark'],
    [null, false, 'light'],
  ])('stored %s with prefers-dark=%s follows the system (%s)', (stored, prefersDark, expected) => {
    if (stored) localStorage.setItem('theme', stored)
    stubSystemTheme(prefersDark)
    loadThemeScript()()
    expect(root.classList.contains(expected)).toBe(true)
    expect(root.classList).toHaveLength(1)
    expect(root.style.colorScheme).toBe(expected)
  })

  it('does nothing and does not throw when storage is unavailable', () => {
    vi.spyOn(localStorage, 'getItem').mockImplementation(() => {
      throw new Error('storage blocked')
    })
    const run = loadThemeScript()
    expect(() => run()).not.toThrow()
    expect(root.classList).toHaveLength(0)
    expect(root.style.colorScheme).toBe('')
  })
})
