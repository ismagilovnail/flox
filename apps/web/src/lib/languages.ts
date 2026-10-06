/** ISO 639-1 language codes for the filter builder's `language` field. The
 * classifier reduces Accept-Language to this lowercase subtag ("ru-RU" -> "ru"),
 * so these are exactly the values a filter can match. Names are not stored:
 * `languageOptions` asks the browser (Intl.DisplayNames) in the interface language. */
export const ALL_LANGUAGE_CODES: string[] = (
  "aa ab ae af ak am an ar as av ay az ba be bg bh bi bm bn bo br bs ca ce ch co cr cs cu cv cy da de dv dz ee el en eo es et eu fa ff fi fj fo fr fy ga gd gl gn gu gv ha he hi ho hr ht hu hy hz ia id ie ig ii ik io is it iu ja jv ka kg ki kj kk kl km kn ko kr ks ku kv kw ky la lb lg li ln lo lt lu lv mg mh mi mk ml mn mr ms mt my na nb nd ne ng nl nn no nr nv ny oc oj om or os pa pi pl ps pt qu rm rn ro ru rw sa sc sd se sg si sk sl sm sn so sq sr ss st su sv sw ta te tg th ti tk tl tn to tr ts tt tw ty ug uk ur uz ve vi vo wa wo xh yi yo za zh zu"
).split(" ");

/** Languages that go first: the ones this team's traffic mostly speaks. */
const PINNED_LANGUAGES = ["ru", "uk", "en", "kk", "be", "uz", "az", "hy", "ka", "ro", "de", "fr", "es", "pt", "it", "pl", "tr", "ar", "hi", "id", "vi", "th", "ja", "ko", "zh"];

/** All languages as picker options ({ value: "ru", label: "RU — русский" }):
 * pinned first, the rest alphabetical by localized name. */
export function languageOptions(locale: string): { value: string; label: string }[] {
  let names: Intl.DisplayNames | null = null;
  try {
    names = new Intl.DisplayNames([locale], { type: "language" });
  } catch {
    // runtime without Intl.DisplayNames: fall back to bare codes
  }
  const name = (code: string) => {
    try {
      return names?.of(code) ?? code;
    } catch {
      return code;
    }
  };
  const pinned = PINNED_LANGUAGES.filter((c) => ALL_LANGUAGE_CODES.includes(c));
  const rest = ALL_LANGUAGE_CODES.filter((c) => !pinned.includes(c)).sort((a, b) => name(a).localeCompare(name(b), locale));
  return [...pinned, ...rest].map((code) => ({ value: code, label: `${code.toUpperCase()} — ${name(code)}` }));
}
