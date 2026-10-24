/**
 * Timezones offered in the report builder and the organization settings
 * (audit UX-4). Pinned groups go first — Russia (the zones media buyers
 * actually report in), the CIS and Turkey, a few world hubs — then every
 * other IANA zone the browser knows, searchable. The backend accepts any
 * IANA zone (apps/api embeds tzdata).
 */

export type TimezoneGroup = "russia" | "cis" | "world" | "all";

export const TIMEZONE_GROUPS: { group: TimezoneGroup; ids: string[] }[] = [
  {
    group: "russia",
    ids: [
      "Europe/Kaliningrad",
      "Europe/Moscow",
      "Europe/Samara",
      "Asia/Yekaterinburg",
      "Asia/Omsk",
      "Asia/Krasnoyarsk",
      "Asia/Novosibirsk",
      "Asia/Irkutsk",
      "Asia/Yakutsk",
      "Asia/Vladivostok",
      "Asia/Magadan",
      "Asia/Kamchatka",
    ],
  },
  {
    group: "cis",
    ids: [
      "Europe/Kyiv",
      "Europe/Minsk",
      "Asia/Almaty",
      "Asia/Tashkent",
      "Asia/Bishkek",
      "Asia/Dushanbe",
      "Asia/Baku",
      "Asia/Yerevan",
      "Asia/Tbilisi",
      "Europe/Chisinau",
      "Europe/Istanbul",
    ],
  },
  {
    group: "world",
    ids: [
      "UTC",
      "Europe/London",
      "Europe/Berlin",
      "Asia/Dubai",
      "Asia/Singapore",
      "America/New_York",
      "America/Chicago",
      "America/Los_Angeles",
    ],
  },
];

/** The pinned zones (TIMEZONE_GROUPS), in order. */
export const TIMEZONE_IDS = TIMEZONE_GROUPS.flatMap((g) => g.ids);

/** Old spellings the browser may still list next to the pinned one. */
const ALIASES: Record<string, string> = { "Europe/Kiev": "Europe/Kyiv" };

/** Every other IANA zone the runtime knows (Intl.supportedValuesOf), sorted;
 * empty where the runtime cannot list them — the pinned ones stay. */
export function otherTimezoneIds(): string[] {
  let all: string[] = [];
  try {
    all = (Intl as unknown as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.("timeZone") ?? [];
  } catch {
    all = [];
  }
  const pinned = new Set(TIMEZONE_IDS);
  return all.filter((tz) => !pinned.has(tz) && !pinned.has(ALIASES[tz] ?? "")).sort();
}

/** "America/Argentina/Buenos_Aires" → "Buenos Aires (America/Argentina)":
 * a readable name for a zone without a translation. */
export function timezoneFallbackName(tz: string): string {
  const parts = tz.split("/");
  if (parts.length < 2) return tz;
  return `${parts[parts.length - 1].replaceAll("_", " ")} (${parts.slice(0, -1).join("/")})`;
}

/** "UTC+3", "UTC-5", "UTC+5:30" for the zone right now; "" for UTC itself or a zone the runtime doesn't know. */
export function utcOffsetLabel(timeZone: string, at: Date = new Date()): string {
  if (timeZone === "UTC") return "";
  try {
    const part = new Intl.DateTimeFormat("en", { timeZone, timeZoneName: "shortOffset" })
      .formatToParts(at)
      .find((p) => p.type === "timeZoneName")?.value;
    if (!part) return "";
    return part === "GMT" ? "" : part.replace("GMT", "UTC").replace("−", "-");
  } catch {
    return "";
  }
}

/** The calendar day it is right now in `timeZone`, as a UTC-midnight Date — the
 * shape the analytics API's day ranges (YYYY-MM-DD via toISOString) expect.
 * An unknown zone falls back to the UTC day. */
export function todayInZone(timeZone: string, at: Date = new Date()): Date {
  try {
    const ymd = new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" }).format(at);
    return new Date(`${ymd}T00:00:00Z`);
  } catch {
    return new Date(`${at.toISOString().slice(0, 10)}T00:00:00Z`);
  }
}

/** How far `timeZone` is ahead of UTC at instant `at`, in minutes. */
function offsetMinutes(timeZone: string, at: Date): number {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-US", {
      timeZone, hourCycle: "h23", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit",
    })
      .formatToParts(at)
      .map((p) => [p.type, p.value]),
  );
  const asUtc = Date.UTC(+parts.year, +parts.month - 1, +parts.day, +parts.hour, +parts.minute, +parts.second);
  return Math.round((asUtc - Math.floor(at.getTime() / 1000) * 1000) / 60000);
}

/** The instant a wall-clock time ("YYYY-MM-DDTHH:mm", a datetime-local
 * input's value) names in `timeZone`. Across a DST change the offset is
 * taken at the result, so a time in the autumn overlap resolves to its
 * first occurrence and one in the spring gap moves forward. */
export function zonedLocalToUtc(local: string, timeZone: string): Date {
  const naive = new Date(`${local}:00Z`);
  const first = new Date(naive.getTime() - offsetMinutes(timeZone, naive) * 60000);
  return new Date(naive.getTime() - offsetMinutes(timeZone, first) * 60000);
}

/** `at` as a wall-clock time in `timeZone`, "YYYY-MM-DDTHH:mm" — the value
 * a datetime-local input takes. */
export function utcToZonedLocal(at: Date, timeZone: string): string {
  const shifted = new Date(at.getTime() + offsetMinutes(timeZone, at) * 60000);
  return shifted.toISOString().slice(0, 16);
}

/** A date and time as the organization reads it, with its zone. */
export function formatInZone(at: Date, timeZone: string, locale: string): string {
  // dateStyle/timeStyle cannot be combined with timeZoneName — the fields
  // are spelled out instead.
  return new Intl.DateTimeFormat(locale, {
    timeZone, year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", timeZoneName: "short",
  }).format(at);
}
