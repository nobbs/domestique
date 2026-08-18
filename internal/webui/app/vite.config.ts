import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The development API from `make dev-api` listens here. The dev server proxies
// to it and forwards a Cloudflare Access assertion, which is the only identity
// the service accepts.
//
// There is deliberately no way to skip that check in development: put a real
// assertion in DOMESTIQUE_DEV_ASSERTION and the gate behaves exactly as it does
// in production. Sign in to the deployed hostname once and copy the
// Cf-Access-Jwt-Assertion header from any request the browser makes; it stays
// valid for the Access session duration. Without it every proxied request
// answers 401, which is the correct answer to a request that proves nothing.
//
// Point DOMESTIQUE_DEV_API at http://127.0.0.1:8080 to work against the
// deployed container instead — but note that one can reach Wahoo, whereas the
// development API deliberately cannot.
const apiTarget = process.env.DOMESTIQUE_DEV_API ?? "http://127.0.0.1:8081";
const devAssertion = process.env.DOMESTIQUE_DEV_ASSERTION;

const proxy = {
  target: apiTarget,
  changeOrigin: false,
  headers: devAssertion ? { "Cf-Access-Jwt-Assertion": devAssertion } : {},
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
