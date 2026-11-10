import type { NextConfig } from "next";

/**
 * The browser never learns where the Go API lives.
 *
 * `/api/*` is proxied to the backend, so the frontend and the API share one
 * origin in development exactly as Caddy makes them share one origin in
 * production. That removes CORS from the picture entirely and keeps session
 * cookies first-party.
 */
const apiOrigin = process.env.API_ORIGIN ?? "http://127.0.0.1:8080";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  output: "standalone",
  typedRoutes: true,
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${apiOrigin}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
