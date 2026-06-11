import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  reactCompiler: true,
  output: 'standalone',
  // Pin the workspace root so multi-lockfile detection doesn't mis-trace the
  // standalone output (important for the Docker build).
  turbopack: {
    root: __dirname,
  },
};

export default nextConfig;
