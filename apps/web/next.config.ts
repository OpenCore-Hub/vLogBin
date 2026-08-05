import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // This app lives in a monorepo subdirectory; pin the workspace root so
  // Next.js does not infer it from lockfiles higher up the tree.
  turbopack: {
    root: import.meta.dirname,
  },
  // Emit a self-contained server bundle so the runtime image only needs
  // node + the standalone output (no node_modules install at runtime).
  output: "standalone",
};

export default nextConfig;
