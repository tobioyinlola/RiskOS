import type { NextConfig } from "next";

// Security headers for every response. A Content-Security-Policy with per-request nonces is
// added with authentication in the frontend phase (TODO(phase-6)); these headers are safe now.
const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=(), payment=()",
  },
];

const nextConfig: NextConfig = {
  // Self-contained server for the distroless runtime image (see frontend/Dockerfile).
  output: "standalone",
  typedRoutes: true,
  poweredByHeader: false,
  reactStrictMode: true,
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
};

export default nextConfig;
