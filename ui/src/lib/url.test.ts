import { describe, it, expect } from 'vitest'
import { isSafeUrl } from './url'

describe('isSafeUrl', () => {
  it.each([
    ['http://jira.example.com/PROJ-1', true],
    ['https://jira.example.com/PROJ-1', true],
    ['javascript:alert(1)', false],
    ['JavaScript:alert(1)', false],
    ['data:text/html,<script>alert(1)</script>', false],
    ['vbscript:MsgBox(1)', false],
    ['ftp://files.example.com/report', false],
    ['jira.example.com/PROJ-1', false],
    ['', false],
  ])('isSafeUrl(%j) is %s', (url, expected) => {
    expect(isSafeUrl(url)).toBe(expected)
  })
})
