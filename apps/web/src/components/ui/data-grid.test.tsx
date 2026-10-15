import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import type { ColumnDef } from "@tanstack/react-table";

import "@/lib/i18n/config";
import { DataGrid, type GridPage } from "@/components/ui/data-grid";
import { collectPages, hasNextPage, pageRange, previousPage } from "@/components/ui/data-grid-paging";
import type { dataTableFeatures } from "@/components/ui/table-parts";

type Row = { id: string; n: number; label: string };

const makeRows = (from: number, count: number): Row[] =>
  Array.from({ length: count }, (_, i) => ({ id: `r${from + i}`, n: from + i, label: `row, "${from + i}"` }));

const columns: ColumnDef<typeof dataTableFeatures, Row>[] = [
  { id: "n", accessorKey: "n", header: "N" },
  { id: "label", accessorKey: "label", header: "Label" },
  { id: "actions", header: "", cell: () => "…" },
];

beforeAll(() => {
  // jsdom has no layout: give elements the size of a real viewport, which
  // the virtualizer reads (offsetHeight/offsetWidth) to pick the rows in view.
  Object.defineProperty(HTMLElement.prototype, "offsetHeight", { configurable: true, get: () => 560 });
  Object.defineProperty(HTMLElement.prototype, "offsetWidth", { configurable: true, get: () => 1200 });
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

afterEach(() => vi.restoreAllMocks());

function renderGrid(props: Partial<React.ComponentProps<typeof DataGrid<Row>>> & { page?: GridPage } = {}) {
  const onPageChange = vi.fn();
  const onRowActivate = vi.fn();
  const page = props.page ?? { offset: 0, limit: 200 };
  const utils = render(
    <DataGrid<Row>
      columns={columns}
      rows={makeRows(page.offset, 200)}
      total={450}
      page={page}
      onPageChange={onPageChange}
      onRowActivate={onRowActivate}
      getRowId={(r) => r.id}
      ariaLabel="Test grid"
      {...props}
    />,
  );
  return { ...utils, onPageChange, onRowActivate };
}

describe("data grid paging", () => {
  it("shows where a page sits in the total", () => {
    expect(pageRange({ offset: 200, limit: 200 }, 50, 250)).toEqual({ first: 201, last: 250 });
    expect(pageRange({ offset: 0, limit: 50 }, 0, 0)).toEqual({ first: 0, last: 0 });
    expect(hasNextPage({ offset: 200, limit: 200 }, 400)).toBe(false);
    expect(hasNextPage({ offset: 200, limit: 200 }, 401)).toBe(true);
    expect(previousPage({ offset: 50, limit: 200 })).toEqual({ offset: 0, limit: 200 });
  });

  it("collects every page, and stops at the cap saying so", async () => {
    const all = makeRows(0, 450);
    const fetchPage = vi.fn(async (offset: number, limit: number) => ({ rows: all.slice(offset, offset + limit), total: all.length }));
    const whole = await collectPages(fetchPage, { pageSize: 200, cap: 10_000 });
    expect(whole.rows.map((r) => r.n)).toEqual(all.map((r) => r.n));
    expect(whole.truncated).toBe(false);
    expect(fetchPage.mock.calls).toEqual([[0, 200], [200, 200], [400, 200]]);

    fetchPage.mockClear();
    const progress: number[] = [];
    const capped = await collectPages(fetchPage, { pageSize: 200, cap: 300, onProgress: (done) => progress.push(done) });
    expect(capped.rows).toHaveLength(300);
    expect(capped.truncated).toBe(true);
    expect(fetchPage.mock.calls).toEqual([[0, 200], [200, 100]]);
    expect(progress).toEqual([200, 300]);
  });

  it("stops at a short page", async () => {
    const fetchPage = vi.fn(async (offset: number) => ({ rows: offset === 0 ? makeRows(0, 120) : [], total: 500 }));
    const got = await collectPages(fetchPage, { pageSize: 200, cap: 10_000 });
    expect(got.rows).toHaveLength(120);
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });
});

describe("DataGrid", () => {
  it("renders only the rows in view, with the server's range", () => {
    renderGrid();
    const body = screen.getByRole("grid", { name: "Test grid" });
    const rendered = within(body).getAllByRole("row").filter((r) => r.hasAttribute("data-index"));
    expect(rendered.length).toBeGreaterThan(10);
    expect(rendered.length).toBeLessThan(60);
    expect(screen.getByText(/1–200 (of|из) 450/)).toBeInTheDocument();
  });

  it("asks the server for the next page and a new page size from the start", () => {
    const { onPageChange } = renderGrid({ page: { offset: 200, limit: 200 } });
    fireEvent.click(screen.getByRole("button", { name: /next page|следующая/i }));
    expect(onPageChange).toHaveBeenLastCalledWith({ offset: 400, limit: 200 });
    fireEvent.click(screen.getByRole("button", { name: /previous page|предыдущая/i }));
    expect(onPageChange).toHaveBeenLastCalledWith({ offset: 0, limit: 200 });
  });

  it("disables next on the last page", () => {
    renderGrid({ page: { offset: 400, limit: 200 }, rows: makeRows(400, 50) });
    expect(screen.getByRole("button", { name: /next page|следующая/i })).toBeDisabled();
    expect(screen.getByText(/401–450/)).toBeInTheDocument();
  });

  it("moves the active row with the keyboard and activates it with Enter", () => {
    const { onRowActivate } = renderGrid();
    const body = screen.getByRole("grid", { name: "Test grid" });
    body.focus();
    fireEvent.keyDown(body, { key: "ArrowDown" });
    fireEvent.keyDown(body, { key: "ArrowDown" });
    expect(body.querySelector('[aria-selected="true"]')?.getAttribute("data-index")).toBe("1");
    expect(body.getAttribute("aria-activedescendant")).toBe("grid-row-1");
    fireEvent.keyDown(body, { key: "ArrowUp" });
    fireEvent.keyDown(body, { key: "Enter" });
    expect(onRowActivate).toHaveBeenCalledWith(expect.objectContaining({ id: "r0" }));
  });

  it("renders rows whose ids collide without losing any", () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    const twins = makeRows(0, 5).map((r) => ({ ...r, id: "same" }));
    renderGrid({ rows: twins, total: 5 });
    const rendered = within(screen.getByRole("grid", { name: "Test grid" })).getAllByRole("row").filter((r) => r.hasAttribute("data-index"));
    expect(rendered).toHaveLength(5);
    expect(errors.mock.calls.some((c) => String(c[0]).includes("same key"))).toBe(false);
  });

  it("shows the empty state with no rows", () => {
    renderGrid({ rows: [], total: 0, emptyTitle: "Nothing logged" });
    expect(screen.getByText("Nothing logged")).toBeInTheDocument();
  });

  it("exports every page up to the cap and warns that it stopped", async () => {
    const all = makeRows(0, 450);
    const fetchPage = vi.fn(async (offset: number, limit: number) => ({ rows: all.slice(offset, offset + limit), total: all.length }));
    let blob: Blob | undefined;
    vi.spyOn(URL, "createObjectURL").mockImplementation((b) => {
      blob = b as Blob;
      return "blob:test";
    });
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    renderGrid({ fetchPage, exportCap: 300, exportFilename: "test" });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /export csv|экспорт/i }));
    });
    const lines = (await blob!.text()).split("\n");
    expect(lines[0]).toBe("N,Label");
    expect(lines).toHaveLength(301);
    expect(lines[1]).toBe('0,"row, ""0"""');
    expect(fetchPage.mock.calls).toEqual([[0, 200], [200, 100]]);
  });
});
