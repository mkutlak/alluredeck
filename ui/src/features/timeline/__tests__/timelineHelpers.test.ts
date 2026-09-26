import { describe, it, expect } from 'vitest'
import { computeTicks, formatRelativeTime } from '../timelineHelpers'

describe('formatRelativeTime', () => {
  it.each([
    [0, '+0s'],
    [999, '+0s'],
    [4500, '+4s'],
    [59000, '+59s'],
    [60000, '+1m'],
    [120000, '+2m'],
    [90000, '+1m 30s'],
    [150000, '+2m 30s'],
  ])('formats %ims as %s', (ms, label) => {
    expect(formatRelativeTime(ms)).toBe(label)
  })
})

describe('computeTicks', () => {
  // The finest "nice" step that keeps the axis at <= 12 ticks, labelled from the range start.
  it.each([
    { name: 'a degenerate range', min: 5000, max: 5000, labels: ['+0s'] },
    {
      name: 'a 20s range in 2s steps',
      min: 1_000_000,
      max: 1_020_000,
      labels: ['+0s', '+2s', '+4s', '+6s', '+8s', '+10s', '+12s', '+14s', '+16s', '+18s', '+20s'],
    },
    {
      name: 'a 10m range in 1m steps',
      min: 0,
      max: 600_000,
      labels: ['+0s', '+1m', '+2m', '+3m', '+4m', '+5m', '+6m', '+7m', '+8m', '+9m', '+10m'],
    },
  ])('labels $name', ({ min, max, labels }) => {
    expect(computeTicks(min, max).map((t) => t.label)).toEqual(labels)
  })
})
