import type { PwaComment } from "@/lib/api/pwa";

/** The 20 countries offered when adding a comment set, each with the
 * languages commonly used there (the first is the default). Any other
 * geo/lang pair still works via CSV import. */
export const COMMENT_GEOS: { geo: string; langs: string[] }[] = [
  { geo: "US", langs: ["en", "es"] },
  { geo: "IN", langs: ["hi", "en"] },
  { geo: "BR", langs: ["pt"] },
  { geo: "ID", langs: ["id", "en"] },
  { geo: "RU", langs: ["ru"] },
  { geo: "MX", langs: ["es"] },
  { geo: "JP", langs: ["ja"] },
  { geo: "DE", langs: ["de"] },
  { geo: "GB", langs: ["en"] },
  { geo: "FR", langs: ["fr"] },
  { geo: "TR", langs: ["tr"] },
  { geo: "IT", langs: ["it"] },
  { geo: "ES", langs: ["es", "ca"] },
  { geo: "PH", langs: ["en", "tl"] },
  { geo: "VN", langs: ["vi"] },
  { geo: "TH", langs: ["th"] },
  { geo: "PL", langs: ["pl"] },
  { geo: "CA", langs: ["en", "fr"] },
  { geo: "EG", langs: ["ar", "en"] },
  { geo: "NG", langs: ["en"] },
];

/** Localised region/language name for a code, falling back to the code. */
export function displayName(type: "region" | "language", code: string, locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type }).of(code) ?? code;
  } catch {
    return code;
  }
}

/** A comment set is one geo + language pair; its key is "GEO-lang". */
export const setKey = (c: Pick<PwaComment, "geo" | "lang">) => `${c.geo}-${c.lang}`;

/** Average and count of a set's own ratings, plus each star's share in
 * percent — undefined when the set is empty, never a made-up zero. */
export function commentStats(comments: PwaComment[]) {
  if (comments.length === 0) return undefined;
  const counts: Record<number, number> = { 1: 0, 2: 0, 3: 0, 4: 0, 5: 0 };
  let sum = 0;
  for (const c of comments) {
    counts[c.rating] += 1;
    sum += c.rating;
  }
  const pct = (n: number) => Math.round((counts[n] / comments.length) * 1000) / 10;
  return {
    rating: Math.round((sum / comments.length) * 10) / 10,
    count: comments.length,
    stars: { 5: pct(5), 4: pct(4), 3: pct(3), 2: pct(2), 1: pct(1) },
  };
}

const CSV_HEADER = ["geo", "lang", "author", "rating", "text"];

const quote = (v: string) => (/[",\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v);

export function commentsToCsv(comments: PwaComment[]): string {
  const rows = comments.map((c) => [c.geo, c.lang, c.author, String(c.rating), c.text].map(quote).join(","));
  return [CSV_HEADER.join(","), ...rows].join("\n");
}

function parseCsvRows(input: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let inQuotes = false;
  for (let i = 0; i < input.length; i++) {
    const ch = input[i];
    if (inQuotes) {
      if (ch === '"' && input[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (ch === '"') {
        inQuotes = false;
      } else {
        cell += ch;
      }
    } else if (ch === '"') {
      inQuotes = true;
    } else if (ch === ",") {
      row.push(cell);
      cell = "";
    } else if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && input[i + 1] === "\n") i++;
      row.push(cell);
      cell = "";
      rows.push(row);
      row = [];
    } else {
      cell += ch;
    }
  }
  if (cell !== "" || row.length > 0) {
    row.push(cell);
    rows.push(row);
  }
  return rows.filter((r) => r.some((c) => c.trim() !== ""));
}

/** Parses an exported/hand-made CSV (geo,lang,author,rating,text with a
 * header row). Rows that don't fit are counted, not guessed at. */
export function parseCommentsCsv(input: string): { comments: PwaComment[]; skipped: number } {
  const rows = parseCsvRows(input.replace(/^﻿/, ""));
  const body = rows[0]?.[0]?.trim().toLowerCase() === "geo" ? rows.slice(1) : rows;
  const comments: PwaComment[] = [];
  let skipped = 0;
  for (const r of body) {
    const [geo = "", lang = "", author = "", rating = "", text = ""] = r.map((c) => c.trim());
    const n = Number(rating);
    if (!/^[A-Za-z]{2}$/.test(geo) || !/^[A-Za-z]{2}$/.test(lang) || !author || !text || !Number.isInteger(n) || n < 1 || n > 5) {
      skipped++;
      continue;
    }
    comments.push({ geo: geo.toUpperCase(), lang: lang.toLowerCase(), author, rating: n, text });
  }
  return { comments, skipped };
}
