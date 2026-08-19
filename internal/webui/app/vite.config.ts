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

// State-changing requests must come from the origin the API is configured to
// serve its UI at, which is the origin of its wahoo.redirect_url. The dev server
// runs on another one, so it names the API's for the requests it proxies. The
// default is what dev/setup.sh writes; set DOMESTIQUE_DEV_ORIGIN when pointing
// DOMESTIQUE_DEV_API at the deployed container instead, whose origin is public.
//
// This is not a way around the check: it is this proxy stating the origin it is
// standing in for, exactly as it states the identity it is standing in for.
const devOrigin = process.env.DOMESTIQUE_DEV_ORIGIN ?? "https://127.0.0.1:9";

const proxy = {
  target: apiTarget,
  changeOrigin: false,
  headers: {
    ...(devAssertion ? { "Cf-Access-Jwt-Assertion": devAssertion } : {}),
    Origin: devOrigin,
  },
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
    // Vitest would otherwise collect e2e/*.spec.ts, which is Playwright's suite
    // and needs a browser. The unit suites stay a jsdom-only run that a
    // contributor with no browser installed can still pass. `scripts` is the
    // coverage tooling rather than the UI, so it is tested here and measured
    // nowhere, exactly as `dev/` is on the Go side.
    include: ["src/**/*.test.{ts,tsx}", "scripts/**/*.test.ts"],
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    restoreMocks: true,
    coverage: {
      provider: "v8",
      // The repository collects Go and UI coverage side by side, so both
      // reports land under one gitignored directory at the repository root.
      reportsDirectory: "../../../.coverage/ui",
      // `json` is what the browser half merges against. LCOV cannot be read back
      // without losing the statement locations the merge aligns on, and it is
      // also this report that tells the browser half which files are measured at
      // all — every file the two lists below match appears in it, covered or
      // not, so the two collectors cannot end up measuring different trees.
      reporter: ["text-summary", "lcov", "json"],
      // Measure the whole application, not only the files a test happened to
      // import: an untested module is the number's point, not an omission.
      // Vitest reports every file this matches, covered or not.
      include: ["src/**/*.{ts,tsx}"],
      exclude: [
        // Test files and their harness are the measurement, not the subject.
        "src/**/*.test.{ts,tsx}",
        "src/test/**",
        // Type-only declarations emit no runtime statement to cover.
        "src/**/*.d.ts",
        // The bootstrap mounts React onto a real document and does nothing
        // else; a test of it would assert the framework, not this UI.
        "src/main.tsx",
      ],
    },
  },
});
