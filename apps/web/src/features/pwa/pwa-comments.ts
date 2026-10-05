import type { PwaComment } from "@/lib/api/pwa";

/** Countries offered when adding a comment set (West/Central Africa and the
 * other common traffic geos), each with the languages commonly used there
 * (the first is the default). Any other geo/lang pair still works via CSV
 * import. */
export const COMMENT_GEOS: { geo: string; langs: string[] }[] = [
  { geo: "ML", langs: ["fr", "bm"] },
  { geo: "BF", langs: ["fr"] },
  { geo: "SN", langs: ["fr", "wo"] },
  { geo: "CI", langs: ["fr"] },
  { geo: "GN", langs: ["fr", "ff"] },
  { geo: "NE", langs: ["fr", "ha"] },
  { geo: "TD", langs: ["fr", "ar"] },
  { geo: "TG", langs: ["fr"] },
  { geo: "BJ", langs: ["fr"] },
  { geo: "CM", langs: ["fr", "en"] },
  { geo: "CD", langs: ["fr", "sw"] },
  { geo: "CG", langs: ["fr"] },
  { geo: "GA", langs: ["fr"] },
  { geo: "MG", langs: ["mg", "fr"] },
  { geo: "DZ", langs: ["ar", "fr"] },
  { geo: "MA", langs: ["ar", "fr"] },
  { geo: "TN", langs: ["ar", "fr"] },
  { geo: "EG", langs: ["ar", "en"] },
  { geo: "NG", langs: ["en", "ha"] },
  { geo: "GH", langs: ["en"] },
  { geo: "KE", langs: ["sw", "en"] },
  { geo: "TZ", langs: ["sw", "en"] },
  { geo: "UG", langs: ["en", "sw"] },
  { geo: "ET", langs: ["am", "en"] },
  { geo: "ZA", langs: ["en"] },
  { geo: "ZM", langs: ["en"] },
  { geo: "ZW", langs: ["en"] },
  { geo: "MZ", langs: ["pt"] },
  { geo: "AO", langs: ["pt"] },
  { geo: "SD", langs: ["ar"] },
  { geo: "SO", langs: ["so", "ar"] },
  { geo: "RW", langs: ["rw", "fr"] },
  { geo: "BI", langs: ["rn", "fr"] },
  { geo: "MR", langs: ["ar", "fr"] },
  { geo: "LY", langs: ["ar"] },
  { geo: "SL", langs: ["en"] },
  { geo: "LR", langs: ["en"] },
  { geo: "GM", langs: ["en"] },
  { geo: "CV", langs: ["pt"] },
  { geo: "GW", langs: ["pt"] },
  { geo: "BW", langs: ["en"] },
  { geo: "NA", langs: ["en"] },
  { geo: "MW", langs: ["en"] },
  { geo: "US", langs: ["en", "es"] },
  { geo: "CA", langs: ["en", "fr"] },
  { geo: "MX", langs: ["es"] },
  { geo: "BR", langs: ["pt"] },
  { geo: "AR", langs: ["es"] },
  { geo: "CL", langs: ["es"] },
  { geo: "CO", langs: ["es"] },
  { geo: "PE", langs: ["es"] },
  { geo: "VE", langs: ["es"] },
  { geo: "EC", langs: ["es"] },
  { geo: "BO", langs: ["es"] },
  { geo: "UY", langs: ["es"] },
  { geo: "PY", langs: ["es"] },
  { geo: "DO", langs: ["es"] },
  { geo: "GT", langs: ["es"] },
  { geo: "CU", langs: ["es"] },
  { geo: "CR", langs: ["es"] },
  { geo: "PA", langs: ["es"] },
  { geo: "HN", langs: ["es"] },
  { geo: "SV", langs: ["es"] },
  { geo: "NI", langs: ["es"] },
  { geo: "PR", langs: ["es", "en"] },
  { geo: "JM", langs: ["en"] },
  { geo: "HT", langs: ["fr"] },
  { geo: "GB", langs: ["en"] },
  { geo: "IE", langs: ["en"] },
  { geo: "FR", langs: ["fr"] },
  { geo: "BE", langs: ["fr", "nl", "de"] },
  { geo: "NL", langs: ["nl"] },
  { geo: "DE", langs: ["de"] },
  { geo: "AT", langs: ["de"] },
  { geo: "CH", langs: ["de", "fr", "it"] },
  { geo: "IT", langs: ["it"] },
  { geo: "ES", langs: ["es", "ca"] },
  { geo: "PT", langs: ["pt"] },
  { geo: "PL", langs: ["pl"] },
  { geo: "CZ", langs: ["cs"] },
  { geo: "SK", langs: ["sk"] },
  { geo: "HU", langs: ["hu"] },
  { geo: "RO", langs: ["ro"] },
  { geo: "BG", langs: ["bg"] },
  { geo: "GR", langs: ["el"] },
  { geo: "RS", langs: ["sr"] },
  { geo: "HR", langs: ["hr"] },
  { geo: "UA", langs: ["uk", "ru"] },
  { geo: "BY", langs: ["be", "ru"] },
  { geo: "MD", langs: ["ro", "ru"] },
  { geo: "SE", langs: ["sv"] },
  { geo: "NO", langs: ["no"] },
  { geo: "DK", langs: ["da"] },
  { geo: "FI", langs: ["fi"] },
  { geo: "LT", langs: ["lt"] },
  { geo: "LV", langs: ["lv"] },
  { geo: "EE", langs: ["et"] },
  { geo: "TR", langs: ["tr"] },
  { geo: "RU", langs: ["ru"] },
  { geo: "KZ", langs: ["kk", "ru"] },
  { geo: "UZ", langs: ["uz", "ru"] },
  { geo: "AZ", langs: ["az"] },
  { geo: "GE", langs: ["ka"] },
  { geo: "AM", langs: ["hy"] },
  { geo: "IN", langs: ["hi", "en"] },
  { geo: "PK", langs: ["ur", "en"] },
  { geo: "BD", langs: ["bn"] },
  { geo: "LK", langs: ["si", "ta"] },
  { geo: "NP", langs: ["ne"] },
  { geo: "ID", langs: ["id", "en"] },
  { geo: "PH", langs: ["en", "tl"] },
  { geo: "VN", langs: ["vi"] },
  { geo: "TH", langs: ["th"] },
  { geo: "MY", langs: ["ms", "en"] },
  { geo: "SG", langs: ["en", "zh"] },
  { geo: "JP", langs: ["ja"] },
  { geo: "KR", langs: ["ko"] },
  { geo: "CN", langs: ["zh"] },
  { geo: "TW", langs: ["zh"] },
  { geo: "HK", langs: ["zh", "en"] },
  { geo: "KH", langs: ["km"] },
  { geo: "MM", langs: ["my"] },
  { geo: "SA", langs: ["ar"] },
  { geo: "AE", langs: ["ar", "en"] },
  { geo: "IL", langs: ["he", "en"] },
  { geo: "IQ", langs: ["ar"] },
  { geo: "IR", langs: ["fa"] },
  { geo: "JO", langs: ["ar"] },
  { geo: "LB", langs: ["ar"] },
  { geo: "KW", langs: ["ar"] },
  { geo: "QA", langs: ["ar"] },
  { geo: "OM", langs: ["ar"] },
  { geo: "AU", langs: ["en"] },
  { geo: "NZ", langs: ["en"] },
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

const CSV_HEADER = ["geo", "lang", "author", "rating", "text", "likes", "date", "reply", "avatar"];

const quote = (v: string) => (/[",\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v);

export function commentsToCsv(comments: PwaComment[]): string {
  const rows = comments.map((c) => [c.geo, c.lang, c.author, String(c.rating), c.text, String(c.likes), c.date, c.reply, c.avatarUrl].map(quote).join(","));
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
    const [geo = "", lang = "", author = "", rating = "", text = "", likes = "", date = "", reply = "", avatar = ""] = r.map((c) => c.trim());
    const n = Number(rating);
    const l = likes === "" ? 0 : Number(likes);
    if (
      !/^[A-Za-z]{2}$/.test(geo) || !/^[A-Za-z]{2}$/.test(lang) || !author || !text || text.length > 500 || reply.length > 500 ||
      !Number.isInteger(n) || n < 1 || n > 5 || !Number.isInteger(l) || l < 0 ||
      (date !== "" && !/^\d{4}-\d{2}-\d{2}$/.test(date)) || (avatar !== "" && !/^https?:\/\//.test(avatar))
    ) {
      skipped++;
      continue;
    }
    comments.push({ geo: geo.toUpperCase(), lang: lang.toLowerCase(), author, avatarUrl: avatar, rating: n, likes: l, date, text, reply });
  }
  return { comments, skipped };
}
