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

export const CURRENCIES = ["USD", "EUR", "GBP", "CAD", "AUD", "SEK", "PLN", "BRL"];
