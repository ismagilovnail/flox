/** The server-paging arithmetic of DataGrid, kept apart so it is tested
 * without rendering. */

export type GridPage = { offset: number; limit: number }

/** 1-based first and last row shown on `page` of `total` (0, 0 when empty). */
export function pageRange(page: GridPage, shown: number, total: number): { first: number; last: number } {
  if (shown === 0 || total === 0) return { first: 0, last: 0 }
  return { first: page.offset + 1, last: Math.min(page.offset + shown, total) }
}

export function hasNextPage(page: GridPage, total: number): boolean {
  return page.offset + page.limit < total
}

/** The page before, never below 0. */
export function previousPage(page: GridPage): GridPage {
  return { ...page, offset: Math.max(0, page.offset - page.limit) }
}

export function nextPage(page: GridPage): GridPage {
  return { ...page, offset: page.offset + page.limit }
}

/** A new page size starts over at the first page: the old offset would land
 * mid-page of the new size. */
export function withPageSize(limit: number): GridPage {
  return { offset: 0, limit }
}

export type FetchPage<T> = (offset: number, limit: number) => Promise<{ rows: T[]; total: number }>

/**
 * Every row the server has for the current filter, page by page, up to
 * `cap` rows: what an export of "the whole range" needs, when the API
 * serves at most `pageSize` at a time. `truncated` says the cap cut it
 * short. Stops early if a page comes back short (rows removed meanwhile).
 * onProgress gets (rows so far, total).
 */
export async function collectPages<T>(
  fetchPage: FetchPage<T>,
  { pageSize, cap, onProgress, signal }: { pageSize: number; cap: number; onProgress?: (done: number, total: number) => void; signal?: AbortSignal },
): Promise<{ rows: T[]; total: number; truncated: boolean }> {
  const rows: T[] = []
  let total = Infinity
  while (rows.length < Math.min(total, cap)) {
    if (signal?.aborted) throw new DOMException("export cancelled", "AbortError")
    const limit = Math.min(pageSize, cap - rows.length)
    const page = await fetchPage(rows.length, limit)
    total = page.total
    rows.push(...page.rows)
    onProgress?.(rows.length, total)
    if (page.rows.length < limit) break
  }
  const kept = rows.slice(0, cap)
  return { rows: kept, total, truncated: total > kept.length }
}
