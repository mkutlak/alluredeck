import { describe, it, expect } from 'vitest'
import { isSafeUrl } from './url'

describe('isSafeUrl', () => {
  it('accepts http URLs', () => {
    expect(isSafeUrl('http://jira.example.com/PROJ-1')).toBe(true)
  })

  it('accepts https URLs', () => {
    expect(isSafeUrl('https://jira.example.com/PROJ-1')).toBe(true)
  })

  it('rejects javascript: scheme', () => {
    expect(isSafeUrl('javascript:alert(1)')).toBe(false)
  })

  it('rejects data: scheme', () => {
    expect(isSafeUrl('data:text/html,<script>alert(1)</script>')).toBe(false)
  })

  it('rejects vbscript: scheme', () => {
    expect(isSafeUrl('vbscript:MsgBox(1)')).toBe(false)
  })

  it('rejects empty string', () => {
    expect(isSafeUrl('')).toBe(false)
  })

  it('rejects bare hostname (no protocol)', () => {
    expect(isSafeUrl('jira.example.com/PROJ-1')).toBe(false)
  })

  it('rejects ftp: scheme', () => {
    expect(isSafeUrl('ftp://files.example.com/report')).toBe(false)
  })

  it('rejects javascript with mixed case', () => {
    expect(isSafeUrl('JavaScript:alert(1)')).toBe(false)
  })
})
