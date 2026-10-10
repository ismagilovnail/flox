import { apiFetch } from "@/lib/api/client";

/**
 * The API's entity lists (offers, networks, landings, ... — Go
 * internal/pagination) return at most PAGE_LIMIT rows per request, with the
 * list's `total`. The app's tables sort, search and paginate client-side
 * and its pickers offer every entity, so a list function reads every page.
 */
export const PAGE_LIMIT = 500;

/** Stops a list that keeps growing while it is read from looping forever. */
const MAX_PAGES = 100;

type Page<K extends string, T> = { [P in K]: T[] } & { total: number };

type Fetch = <R>(path: string, options: { searchParams: Record<string, string> }) => Promise<R>;

/**
 * Reads `path` page by page into one `{ [key]: T[], total }`. A row that
 * moves between pages while they are read (one created or deleted
 * meanwhile) can show up twice; it is kept once, by id.
 */
export async function fetchAllPages<K extends string, T extends { id: string }>(
  path: string,
  key: K,
  fetchPage: Fetch = apiFetch,
): Promise<Page<K, T>> {
  const byId = new Map<string, T>();
  let offset = 0;
  let total = 0;
  for (let i = 0; i < MAX_PAGES; i++) {
    const page = await fetchPage<Page<K, T>>(path, {
      searchParams: { limit: String(PAGE_LIMIT), offset: String(offset) },
    });
    const rows = page[key];
    total = page.total;
    for (const row of rows) byId.set(row.id, row);
    offset += rows.length;
    if (rows.length === 0 || offset >= total) break;
  }
  return { [key]: [...byId.values()], total } as Page<K, T>;
}
