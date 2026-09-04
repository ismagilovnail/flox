import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

const CYRILLIC_TO_LATIN: Record<string, string> = {
  а: "a", б: "b", в: "v", г: "g", д: "d", е: "e", ё: "e", ж: "zh", з: "z",
  и: "i", й: "y", к: "k", л: "l", м: "m", н: "n", о: "o", п: "p", р: "r",
  с: "s", т: "t", у: "u", ф: "f", х: "h", ц: "ts", ч: "ch", ш: "sh", щ: "sch",
  ъ: "", ы: "y", ь: "", э: "e", ю: "yu", я: "ya",
}

/** URL-safe slug for auto-derived hosted asset URLs (internal landings,
 * tracking links, etc). Transliterates Cyrillic first — otherwise every
 * Cyrillic input (this is a Russian-market product; campaign/landing names
 * are routinely Cyrillic) strips to nothing and always falls back to
 * "untitled". */
export function slugify(input: string) {
  const transliterated = input
    .toLowerCase()
    .replace(/[а-яё]/g, (ch) => CYRILLIC_TO_LATIN[ch] ?? ch)
  return transliterated
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "") || "untitled"
}
