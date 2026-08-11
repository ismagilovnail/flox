/**
 * Locale pinned to "en-US" everywhere — leaving it undefined resolves to the
 * runtime's default locale (server or browser), which silently produces
 * "14 655,87 $" instead of "$14,655.87" depending on where the app runs.
 */
export function formatUsd(n: number, maximumFractionDigits = 0) {
  return n.toLocaleString("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits,
  });
}

export function formatInt(n: number) {
  return Math.round(n).toLocaleString("en-US");
}
