import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

// Contrast guards for the theme tokens in index.css, computed the WCAG 2.x way. Read from disk
// (relative to ui/, where vitest runs): vitest blanks CSS imports, even `?raw`, to an empty string.
// Not import.meta.url: under jsdom it is not a file: URL, and src/test/setup.ts needs jsdom's window.
const indexCss = readFileSync('src/index.css', 'utf8')

type Triple = [h: number, s: number, l: number]

function tokens(block: RegExp): Record<string, Triple> {
  const body = block.exec(indexCss)?.[1]
  expect(body, `index.css has no block matching ${String(block)}`).toBeDefined()
  const out: Record<string, Triple> = {}
  for (const m of (body ?? '').matchAll(/--([\w-]+):\s*([\d.]+)\s+([\d.]+)%\s+([\d.]+)%;/g)) {
    out[m[1] ?? ''] = [Number(m[2]), Number(m[3]), Number(m[4])]
  }
  return out
}

function luminance([h, s, l]: Triple): number {
  const sat = s / 100
  const light = l / 100
  const a = sat * Math.min(light, 1 - light)
  const channel = (n: number) => {
    const k = (n + h / 30) % 12
    const c = light - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * channel(0) + 0.7152 * channel(8) + 0.0722 * channel(4)
}

function contrast(fg: Triple, bg: Triple): number {
  const [hi, lo] = [luminance(fg), luminance(bg)].sort((x, y) => y - x)
  return ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05)
}

const themes = {
  light: tokens(/:root\s*\{([^}]*)\}/),
  dark: tokens(/\.dark\s*\{([^}]*)\}/),
}

function token(theme: Record<string, Triple>, name: string): Triple {
  const value = theme[name]
  expect(value, `--${name} is missing`).toBeDefined()
  return value as Triple
}

describe.each(Object.entries(themes))('%s theme tokens', (_name, theme) => {
  // WCAG 1.4.11: a focus ring is a UI component boundary, so it needs 3:1 on every surface.
  it.each(['ring', 'sidebar-ring'])(
    '--%s is at least 3:1 on the background, card and sidebar',
    (ring) => {
      for (const surface of ['background', 'card', 'sidebar']) {
        expect(
          contrast(token(theme, ring), token(theme, surface)),
          `--${ring} on --${surface}`,
        ).toBeGreaterThanOrEqual(3)
      }
    },
  )

  // --fact carries the facts a developer scans (branch, counts, errors): body-text contrast.
  it.each(['background', 'card'])('--fact is at least 4.5:1 on the %s', (surface) => {
    expect(contrast(token(theme, 'fact'), token(theme, surface))).toBeGreaterThanOrEqual(4.5)
  })
})
