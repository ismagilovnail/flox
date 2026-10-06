/**
 * Timezones offered in the report builder and the organization settings.
 * Curated, not exhaustive: the backend accepts any IANA zone. Russia is its
 * own group (the zones media buyers actually report in); none of these
 * observe DST today, so the offsets are stable.
 */

export type TimezoneGroup = "russia" | "world";

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

export const TIMEZONE_IDS = TIMEZONE_GROUPS.flatMap((g) => g.ids);

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
