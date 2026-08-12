/**
 * PWAs (§28) — real, team-managed entities. Fields mirror the actual Web App
 * Manifest (name/short_name/theme_color/background_color/icon/start_url) so
 * the editor's "Manifest preview" is a direct projection, not a fiction.
 * `bounceInAppWebview` is the §73-required, provider-neutral capability:
 * bounce in-app WebView traffic (FB/IG/TikTok/Telegram) to the external
 * browser so the install prompt can fire — NOT vendor moderator detection,
 * which §73 forbids. IDs match the ones already baked into stream-set flows.
 */

export type PwaStatus = "active" | "paused" | "archived";

export type Pwa = {
  id: string;
  name: string;
  shortName: string;
  themeColor: string;
  backgroundColor: string;
  iconUrl: string;
  startUrl: string;
  bounceInAppWebview: boolean;
  status: PwaStatus;
  createdAt: string;
  updatedAt: string;
};

export const PWAS: Pwa[] = [
  {
    id: "pwa_sweeps",
    name: "Sweeps PWA",
    shortName: "Sweeps",
    themeColor: "#16a34a",
    backgroundColor: "#0a0a0a",
    iconUrl: "https://cdn.floxlink.io/pwa/sweeps/icon-512.png",
    startUrl: "/install/sweeps",
    bounceInAppWebview: true,
    status: "active",
    createdAt: "2026-02-20T00:00:00Z",
    updatedAt: "2026-07-11T00:00:00Z",
  },
  {
    id: "pwa_casino",
    name: "Casino Lite PWA",
    shortName: "CasinoLite",
    themeColor: "#f59e0b",
    backgroundColor: "#111111",
    iconUrl: "https://cdn.floxlink.io/pwa/casino/icon-512.png",
    startUrl: "/install/casino",
    bounceInAppWebview: true,
    status: "active",
    createdAt: "2026-03-18T00:00:00Z",
    updatedAt: "2026-06-25T00:00:00Z",
  },
];
