import { describe, expect, it } from "vitest";

import i18next, { NAMESPACES } from "@/lib/i18n/config";

type Tree = { [key: string]: string | Tree };

const PLURAL = /_(zero|one|two|few|many|other)$/;

/** Every leaf key, with plural suffixes folded into one key ("count_one" →
 * "count"), since English and Russian have different plural forms. */
function keys(tree: Tree, prefix = ""): Set<string> {
  const out = new Set<string>();
  for (const [k, v] of Object.entries(tree)) {
    const path = prefix + k.replace(PLURAL, "");
    if (typeof v === "string") out.add(path);
    else for (const sub of keys(v, path + ".")) out.add(sub);
  }
  return out;
}

function strings(tree: Tree, prefix = ""): [string, string][] {
  return Object.entries(tree).flatMap(([k, v]) =>
    typeof v === "string" ? [[prefix + k, v] as [string, string]] : strings(v, prefix + k + "."),
  );
}

const bundle = (lang: string, ns: string) => (i18next.getResourceBundle(lang, ns) ?? {}) as Tree;

describe("locales", () => {
  it.each(NAMESPACES)("%s has every English key in Russian and the other way round", (ns) => {
    const en = keys(bundle("en", ns));
    const ru = keys(bundle("ru", ns));
    expect(en.size).toBeGreaterThan(0);
    expect([...en].filter((k) => !ru.has(k))).toEqual([]);
    expect([...ru].filter((k) => !en.has(k))).toEqual([]);
  });

  // Marketer audit #6: phase numbers and spec references are for the team,
  // not for users.
  it.each(NAMESPACES)("%s has no phase numbers or spec references", (ns) => {
    for (const lang of ["en", "ru"]) {
      const marked = strings(bundle(lang, ns)).filter(([, v]) => /§|\b[Pp]hase \d|[Фф]аз[еаыу] \d/.test(v));
      expect(marked).toEqual([]);
    }
  });
});
