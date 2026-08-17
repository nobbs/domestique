import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The development API from `make dev-api` listens here. The dev server proxies
// to it and injects the Tailnet identity header that `tailscale serve` supplies
// in production, so the identity gate behaves the same in both.
//
// Point DOMESTIQUE_DEV_API at http://127.0.0.1:8080 to work against the
// deployed container instead — but note that one can reach Wahoo, whereas the
// development API deliberately cannot.
const apiTarget = process.env.DOMESTIQUE_DEV_API ?? "http://127.0.0.1:8081";
const devLogin = process.env.DOMESTIQUE_DEV_LOGIN ?? "rider@example.ts.net";

const proxy = {
  target: apiTarget,
  changeOrigin: false,
  headers: { "Tailscale-User-Login": devLogin },
};

export default defineConfig({
  plugins: [react()],
  // MapLibre instantiates its worker with `{ type: "module" }`, so the worker
  // bundle has to be emitted as an ES module rather than the default IIFE.
  worker: { format: "es" },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // Fail the build rather than silently shipping an oversized bundle.
    chunkSizeWarningLimit: 1200,
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/v1": proxy,
      "/oauth": proxy,
      "/healthz": proxy,
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    restoreMocks: true,
  },
});
