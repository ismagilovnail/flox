import type { NextConfig } from "next";

/**
 * Security headers on every page (tech audit S2). The concrete risk was
 * clickjacking: any site could frame the app and trick a signed-in user
 * into a destructive click. frame-ancestors 'none' (and X-Frame-Options
 * for older browsers) forbids framing; object-src 'none' forbids plugins.
 *
 * No script-src/style-src/base-uri: the landing editor previews a landing
 * in an <iframe srcDoc>, and a srcdoc document inherits this page's CSP —
 * the landing's own scripts and its <base href> to the asset CDN would be
 * blocked. A nonce-based script policy needs that preview on its own URL
 * first (docs/security.md).
 *
 * HSTS only in the built app: browsers ignore it over plain HTTP, and on
 * HTTPS it makes the browser refuse plain HTTP to this host for a year.
 */
const securityHeaders = [
  { key: "Content-Security-Policy", value: "frame-ancestors 'none'; object-src 'none'" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
  ...(process.env.NODE_ENV === "production" ? [{ key: "Strict-Transport-Security", value: "max-age=31536000" }] : []),
];

const nextConfig: NextConfig = {
  // §61/Phase 33: only the server + the modules it actually imports get
  // copied into .next/standalone, so the production Docker image
  // (apps/web/Dockerfile) doesn't need node_modules installed at runtime.
  output: "standalone",

  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
};

export default nextConfig;
