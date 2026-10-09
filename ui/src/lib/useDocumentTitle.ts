import { useEffect } from 'react'

/**
 * Sets the tab's title while the caller is mounted (nothing while undefined), and
 * restores the previous title on change and on unmount.
 * One caller per route level: a nested pair would fight over which title to restore.
 */
export function useDocumentTitle(title: string | undefined) {
  useEffect(() => {
    if (title === undefined) return
    const previous = document.title
    document.title = title
    return () => {
      document.title = previous
    }
  }, [title])
}
