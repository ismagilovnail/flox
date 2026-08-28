import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // §61/Phase 33: only the server + the modules it actually imports get
  // copied into .next/standalone, so the production Docker image
  // (apps/web/Dockerfile) doesn't need node_modules installed at runtime.
  output: "standalone",
};

export default nextConfig;
