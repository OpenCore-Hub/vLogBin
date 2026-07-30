import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // This app lives in a monorepo subdirectory; pin the workspace root so
  // Next.js does not infer it from lockfiles higher up the tree.
  turbopack: {
    root: import.meta.dirname,
  },
};

export default nextConfig;
