import type { NextConfig } from "next";

const enableExport = process.env.NEXT_OUTPUT_EXPORT === "1";

/**
 * Production: `NEXT_OUTPUT_EXPORT=1 next build` → static files for Go embed.
 * Development: normal Next server so catch-all SPA routes work (/login, /projects/:id, …).
 */
const nextConfig: NextConfig = {
  ...(enableExport
    ? {
        output: "export" as const,
        images: { unoptimized: true },
      }
    : {
        images: { unoptimized: true },
      }),
  trailingSlash: false,
  allowedDevOrigins: ["127.0.0.1", "localhost"],
};

export default nextConfig;
