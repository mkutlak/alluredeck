import { describe, it, expect } from 'vitest'
import {
  ACCENT_BADGE_CLASSES,
  ACCENT_TEXT_CLASSES,
  INFO_BADGE_CLASSES,
  NEUTRAL_BADGE_CLASSES,
  DEFECT_CATEGORY_BADGE_CLASSES,
} from '../status-colors'

describe('ACCENT_BADGE_CLASSES', () => {
  it('contains light and dark classes with the mauve hues, tinted not solid', () => {
    expect(ACCENT_BADGE_CLASSES).toContain('#8839ef')
    expect(ACCENT_BADGE_CLASSES).toContain('#cba6f7')
    expect(ACCENT_BADGE_CLASSES).toContain('dark:')
    expect(ACCENT_BADGE_CLASSES).toContain('border-transparent')
    expect(ACCENT_BADGE_CLASSES).toMatch(/bg-\[#[0-9a-f]{6}\]\/15/)
  })
})

describe('ACCENT_TEXT_CLASSES', () => {
  it('contains light and dark mauve text classes', () => {
    expect(ACCENT_TEXT_CLASSES).toContain('#8839ef')
    expect(ACCENT_TEXT_CLASSES).toContain('dark:text-[#cba6f7]')
  })
})

describe('INFO_BADGE_CLASSES', () => {
  it('contains light and dark classes with the info blue hues', () => {
    expect(INFO_BADGE_CLASSES).toContain('#1e66f5')
    expect(INFO_BADGE_CLASSES).toContain('#89b4fa')
    expect(INFO_BADGE_CLASSES).toContain('dark:')
  })
})

describe('NEUTRAL_BADGE_CLASSES', () => {
  it('reuses the skipped overlay grays for light and dark', () => {
    expect(NEUTRAL_BADGE_CLASSES).toContain('#8c8fa1')
    expect(NEUTRAL_BADGE_CLASSES).toContain('dark:')
  })
})

describe('DEFECT_CATEGORY_BADGE_CLASSES', () => {
  it('defines all four defect categories', () => {
    expect(Object.keys(DEFECT_CATEGORY_BADGE_CLASSES).sort()).toEqual(
      ['infrastructure', 'product_bug', 'test_bug', 'to_investigate'].sort(),
    )
  })

  it('maps product_bug to the failed red tint', () => {
    expect(DEFECT_CATEGORY_BADGE_CLASSES.product_bug).toContain('#d20f39')
    expect(DEFECT_CATEGORY_BADGE_CLASSES.product_bug).toContain('dark:')
  })

  it('maps test_bug to the broken peach tint', () => {
    expect(DEFECT_CATEGORY_BADGE_CLASSES.test_bug).toContain('#fe640b')
  })

  it('maps infrastructure to the info blue tint', () => {
    expect(DEFECT_CATEGORY_BADGE_CLASSES.infrastructure).toContain('#1e66f5')
  })

  it('maps to_investigate to the neutral gray tint', () => {
    expect(DEFECT_CATEGORY_BADGE_CLASSES.to_investigate).toContain('#8c8fa1')
  })

  it('is tinted (bg opacity + border-transparent), not solid', () => {
    for (const cls of Object.values(DEFECT_CATEGORY_BADGE_CLASSES)) {
      expect(cls).toContain('border-transparent')
      expect(cls).toMatch(/bg-\[#[0-9a-f]{6}\]\/15/)
    }
  })
})
