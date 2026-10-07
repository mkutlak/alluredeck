import { useSearchParams } from 'react-router'

import { useUIStore } from '@/store/ui'

interface FeedParams {
  branch?: string
  groupIds: number[]
  page?: number
}

/** `group=all`: every group, said explicitly so the URL never falls back to saved groups. */
const ALL_GROUPS = 'all'

const positiveInt = (raw: string | null) => {
  const n = raw !== null && /^\d+$/.test(raw) ? Number(raw) : NaN
  return Number.isSafeInteger(n) && n > 0 ? n : undefined
}

/**
 * The feed's filters live in the URL, `/?branch=…&group=…&group=…&page=…`, so Back
 * from a run page and a shared link land on the same view. A URL that arrives bare
 * (`/`) shows the groups saved in the UI store (rewritten on every group change) and
 * all branches. The global `selectedBranch` is left to the project pages.
 */
export function useFeedParams() {
  const [params, setParams] = useSearchParams()
  const savedGroupIds = useUIStore((s) => s.runsFeedGroupIds)
  const saveGroupIds = useUIStore((s) => s.setRunsFeedGroupIds)

  const branch = params.get('branch') || undefined
  const rawGroups = params.getAll('group')
  const groupIds =
    params.toString() === ''
      ? savedGroupIds
      : rawGroups.includes(ALL_GROUPS)
        ? []
        : [...new Set(rawGroups.map(positiveInt))].filter((id) => id !== undefined)
  const page = positiveInt(params.get('page')) ?? 1

  // Filter changes replace the history entry, so Back skips them; page 1 is left
  // out of the URL, which is how a branch or group change starts over at page 1.
  // `saved` is the saved selection after this change.
  const write = (next: FeedParams, saved = savedGroupIds) => {
    const out = new URLSearchParams()
    if (next.branch) out.set('branch', next.branch)
    for (const id of next.groupIds) out.append('group', String(id))
    // Without the marker, a URL that loses its last param (clearing the branch,
    // back to page 1) would arrive bare and narrow the feed to the saved groups.
    if (next.groupIds.length === 0 && saved.length > 0) out.append('group', ALL_GROUPS)
    if (next.page && next.page > 1) out.set('page', String(next.page))
    setParams(out, { replace: true })
  }

  return {
    branch,
    groupIds,
    page,
    isFiltered: branch !== undefined || groupIds.length > 0,
    /** Every run: past the saved groups that a bare `/` would bring back. */
    allRunsHref: savedGroupIds.length > 0 ? `/?group=${ALL_GROUPS}` : '/',
    setBranch: (b: string | undefined) => write({ branch: b, groupIds }),
    setGroupIds: (ids: number[]) => {
      saveGroupIds(ids)
      write({ branch, groupIds: ids }, ids)
    },
    setPage: (p: number) => write({ branch, groupIds, page: p }),
  }
}
