import { describe, expect, it } from "vitest";

import { fetchAllPages, PAGE_LIMIT } from "@/lib/api/pagination";

type Row = { id: string };

function server(rows: Row[], pageSize = PAGE_LIMIT) {
  const calls: { limit: string; offset: string }[] = [];
  const fetchPage = async <R,>(_path: string, { searchParams }: { searchParams: Record<string, string> }) => {
    calls.push({ limit: searchParams.limit, offset: searchParams.offset });
    const offset = Number(searchParams.offset);
    const size = Math.min(Number(searchParams.limit), pageSize);
    return { offers: rows.slice(offset, offset + size), total: rows.length } as R;
  };
  return { calls, fetchPage };
}

const rows = (n: number) => Array.from({ length: n }, (_, i) => ({ id: `o${i}` }));

describe("fetchAllPages", () => {
  it("reads one page when everything fits", async () => {
    const { calls, fetchPage } = server(rows(3));
    const got = await fetchAllPages<"offers", Row>("/offers", "offers", fetchPage);
    expect(got.offers).toHaveLength(3);
    expect(got.total).toBe(3);
    expect(calls).toEqual([{ limit: String(PAGE_LIMIT), offset: "0" }]);
  });

  it("reads every page past the limit", async () => {
    const { calls, fetchPage } = server(rows(1203));
    const got = await fetchAllPages<"offers", Row>("/offers", "offers", fetchPage);
    expect(got.offers).toHaveLength(1203);
    expect(calls.map((c) => c.offset)).toEqual(["0", "500", "1000"]);
  });

  it("follows the rows the server returned, not the limit asked for", async () => {
    const { calls, fetchPage } = server(rows(5), 2);
    const got = await fetchAllPages<"offers", Row>("/offers", "offers", fetchPage);
    expect(got.offers.map((r) => r.id)).toEqual(["o0", "o1", "o2", "o3", "o4"]);
    expect(calls.map((c) => c.offset)).toEqual(["0", "2", "4"]);
  });

  it("keeps a row that shifted onto the next page once", async () => {
    let call = 0;
    const fetchPage = async <R,>() => {
      call++;
      // A row was created before the second read: o1 shows up again.
      return (call === 1
        ? { offers: [{ id: "o0" }, { id: "o1" }], total: 3 }
        : { offers: [{ id: "o1" }, { id: "o2" }], total: 4 }) as R;
    };
    const got = await fetchAllPages<"offers", Row>("/offers", "offers", fetchPage);
    expect(got.offers.map((r) => r.id)).toEqual(["o0", "o1", "o2"]);
  });

  it("stops on an empty page even if the total says more", async () => {
    const fetchPage = async <R,>() => ({ offers: [], total: 10 }) as R;
    const got = await fetchAllPages<"offers", Row>("/offers", "offers", fetchPage);
    expect(got.offers).toEqual([]);
  });
});
