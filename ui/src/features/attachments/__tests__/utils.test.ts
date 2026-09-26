import { describe, it, expect } from 'vitest'

import { filterAttachments, type MimeFilter } from '../utils'
import type { AttachmentEntry } from '@/types/api'

function attachment(name: string, mime_type: string): AttachmentEntry {
  return { id: 1, name, source: name, mime_type, size_bytes: 1, url: `/mock/${name}` }
}

const all = [
  attachment('screenshot.png', 'image/png'),
  attachment('response.json', 'application/json'),
  attachment('output.txt', 'text/plain'),
  attachment('trace-chromium.zip', 'application/zip'),
  attachment('data.bin', 'application/octet-stream'),
]

describe('filterAttachments', () => {
  it.each<{ filter: MimeFilter; want: string[] }>([
    { filter: '', want: all.map((a) => a.name) },
    { filter: 'image', want: ['screenshot.png'] },
    // Logs cover text and JSON attachments.
    { filter: 'text', want: ['response.json', 'output.txt'] },
    { filter: 'trace', want: ['trace-chromium.zip'] },
    // Other excludes images, logs, and traces.
    { filter: 'other', want: ['data.bin'] },
  ])('filter $filter keeps $want', ({ filter, want }) => {
    expect(filterAttachments(all, filter).map((a) => a.name)).toEqual(want)
  })
})
