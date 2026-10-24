/** Static ISO-3166 country reference for GEO pickers (offer targeting, filter
 * builder values, future directory stats). Common CPA/CPL markets first. */
export type CountryOption = { code: string; name: string };

export const COUNTRIES: CountryOption[] = [
  { code: "US", name: "United States" },
  { code: "GB", name: "United Kingdom" },
  { code: "DE", name: "Germany" },
  { code: "FR", name: "France" },
  { code: "CA", name: "Canada" },
  { code: "AU", name: "Australia" },
  { code: "BR", name: "Brazil" },
  { code: "IN", name: "India" },
  { code: "ES", name: "Spain" },
  { code: "IT", name: "Italy" },
  { code: "NL", name: "Netherlands" },
  { code: "SE", name: "Sweden" },
  { code: "PL", name: "Poland" },
  { code: "MX", name: "Mexico" },
  { code: "JP", name: "Japan" },
  { code: "AE", name: "United Arab Emirates" },
  { code: "SA", name: "Saudi Arabia" },
  { code: "ZA", name: "South Africa" },
  { code: "NG", name: "Nigeria" },
  { code: "PH", name: "Philippines" },
  { code: "ID", name: "Indonesia" },
  { code: "VN", name: "Vietnam" },
  { code: "TH", name: "Thailand" },
  { code: "TR", name: "Turkey" },
  { code: "AR", name: "Argentina" },
  { code: "CO", name: "Colombia" },
  { code: "CH", name: "Switzerland" },
  { code: "AT", name: "Austria" },
  { code: "BE", name: "Belgium" },
  { code: "PT", name: "Portugal" },
  { code: "NZ", name: "New Zealand" },
  { code: "KR", name: "South Korea" },
];

/** Every currency FLOX has reference rates for (docs/fx.md): the ECB's and,
 * for the rouble and CIS currencies, the Bank of Russia's (audit DATA-8).
 * The server takes any ISO 4217 code; one outside this list gets no USD
 * value. */
export const CURRENCIES = [
  "USD", "EUR", "RUB", "AED", "AMD", "AUD", "AZN", "BDT", "BHD", "BOB", "BRL", "BYN", "CAD", "CHF", "CNY",
  "CUP", "CZK", "DKK", "DZD", "EGP", "ETB", "GBP", "GEL", "HKD", "HUF", "IDR", "ILS", "INR", "IRR", "ISK",
  "JPY", "KGS", "KRW", "KZT", "MDL", "MMK", "MNT", "MXN", "MYR", "NGN", "NOK", "NZD", "OMR", "PHP", "PLN",
  "QAR", "RON", "RSD", "SAR", "SEK", "SGD", "THB", "TJS", "TMT", "TRY", "UAH", "UZS", "VND", "ZAR",
];

/** Every ISO-3166-1 alpha-2 code. Names are not stored: `countryOptions` asks the
 * browser (Intl.DisplayNames) for them in the interface language. */
export const ALL_COUNTRY_CODES: string[] = (
  "AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW"
).split(" ");

/** Markets that go first in a country picker: the CIS ones this team trades,
 * then the common CPA/CPL markets from COUNTRIES. */
const PINNED_CODES = ["RU", "UA", "KZ", "BY", "UZ", "AZ", "AM", "GE", "MD", ...COUNTRIES.map((c) => c.code)];

/** All countries as picker options ({ value: "RU", label: "RU — Россия" }):
 * pinned markets first, the rest alphabetical by localized name. */
export function countryOptions(language: string): { value: string; label: string }[] {
  let names: Intl.DisplayNames | null = null;
  try {
    names = new Intl.DisplayNames([language], { type: "region" });
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
  const pinned = PINNED_CODES.filter((c, i) => PINNED_CODES.indexOf(c) === i && ALL_COUNTRY_CODES.includes(c));
  const rest = ALL_COUNTRY_CODES.filter((c) => !pinned.includes(c)).sort((a, b) => name(a).localeCompare(name(b), language));
  return [...pinned, ...rest].map((code) => ({ value: code, label: `${code} — ${name(code)}` }));
}
