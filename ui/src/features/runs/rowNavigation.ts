import type { KeyboardEvent } from 'react'

/** Where j and k are typing, not navigation. */
const TYPING =
  'input, textarea, select, [role="combobox"], [contenteditable]:not([contenteditable="false"])'

/**
 * The feed list's keydown handler: j and k move focus to the next and previous row's
 * target (`data-row-link`: the branch link, or the toggle of a row without one), from
 * anywhere in a row; Enter then opens it as any link does. Only while focus is in the
 * list, and never for typing or modified keys. Rows are the list's children.
 */
export function navigateRows(e: KeyboardEvent<HTMLElement>) {
  // Caps Lock gives "J" without Shift; Shift+J is left to other shortcuts.
  const key = e.key.toLowerCase()
  if ((key !== 'j' && key !== 'k') || e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return
  const list = e.currentTarget
  const target = e.target as Element
  if (target.matches(TYPING)) return

  // A portal in a row (a tooltip, a popover) bubbles its keys here through React, but
  // it is outside the list in the DOM, so it has no row.
  const rows = Array.from(list.children)
  const i = rows.findIndex((r) => r.contains(target))
  if (i === -1) return
  // Consumed even at the ends, or a browser's quick find would take the key.
  e.preventDefault()
  // One target per row, its first: RunRow's own link or toggle comes before the row's
  // content, which may mark links of its own (AI-summary markdown keeps data-* attributes).
  const targets = rows.map((r) => r.querySelector<HTMLElement>('[data-row-link]'))
  const step = key === 'j' ? 1 : -1
  let n = i + step
  while (n >= 0 && n < rows.length && !targets[n]) n += step
  targets[n]?.focus()
}
