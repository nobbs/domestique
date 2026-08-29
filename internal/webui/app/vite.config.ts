/// <reference types="vitest/config" />
import path from "node:path";
import { fileURLToPath, URL } from "node:url";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

const dirname = fileURLToPath(new URL(".", import.meta.url));

// The development API from `mise run dev-api` listens here. The dev server proxies
// to it and forwards a Cloudflare Access assertion, the only identity the service
// accepts; put a real one in DOMESTIQUE_DEV_ASSERTION, copied from a request the
// browser makes against the deployed hostname. DOMESTIQUE_DEV_API can point at
// the deployed container instead, which unlike the development API reaches Wahoo.
const apiTarget = process.env.DOMESTIQUE_DEV_API ?? "http://127.0.0.1:8081";
const devAssertion = process.env.DOMESTIQUE_DEV_ASSERTION;

// State-changing requests must come from the origin the API serves its UI at. The
// dev server runs on another, so it names the API's for the requests it proxies.
// Set DOMESTIQUE_DEV_ORIGIN when pointing DOMESTIQUE_DEV_API at the deployed
// container. This is the proxy stating the origin it stands in for, not a bypass.
const devOrigin = process.env.DOMESTIQUE_DEV_ORIGIN ?? "https://127.0.0.1:9";

// `dev/demo.sh --tailnet` fronts the dev server with `tailscale serve`, which
// forwards a tailnet connection to a loopback address. Two things change: the
// Host arrives as this machine's MagicDNS name, which Vite does not trust on its
// own, so the tailnet suffix is named here; and `localhost` resolves to ::1 while
// Serve forwards to 127.0.0.1, so the address is spelled out literally.
const tailnet = process.env.DOMESTIQUE_DEV_TAILNET === "true";

const proxy = {
  target: apiTarget,
  changeOrigin: false,
  headers: {
    ...(devAssertion ? { "Cf-Access-Jwt-Assertion": devAssertion } : {}),
    Origin: devOrigin,
  },
};

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  // MapLibre instantiates its worker with `{ type: "module" }`, so the worker
  // bundle has to be emitted as an ES module rather than the default IIFE.
  worker: { format: "es" },
  build: {
    // One level below the embedded root. `emptyOutDir` clears what it is pointed
    // at, and the directory Go embeds holds a committed placeholder keeping the
    // embed pattern valid, so the two cannot be the same directory.
    outDir: "dist/bundle",
    emptyOutDir: true,
    // Fail the build rather than silently shipping an oversized bundle.
    chunkSizeWarningLimit: 1200,
  },
  server: {
    port: 5173,
    strictPort: true,
    ...(tailnet ? { host: "127.0.0.1", allowedHosts: [".ts.net"] } : {}),
    proxy: {
      "/v1": proxy,
      "/oauth": proxy,
      "/healthz": proxy,
    },
  },
  test: {
    // The terminal summary a contributor reads and the JUnit report CI uploads,
    // written on every run so the local file is the one the comment came from.
    // `reporters`/`outputFile` are root-level: Vitest applies them to the whole
    // invocation, and VITEST_JUNIT_SUFFIX keeps two task runs from clobbering.
    reporters: ["default", "junit"],
    outputFile: {
      junit: `../../../.test-results/ui/${process.env.VITEST_JUNIT_SUFFIX ?? "vitest"}.xml`,
    },
    coverage: {
      provider: "v8",
      // The repository collects Go and UI coverage side by side, so both
      // reports land under one gitignored directory at the repository root.
      reportsDirectory: "../../../.coverage/ui",
      // The summary a contributor reads, and the LCOV file CI uploads and
      // `dev/patchcoverage` reads. There was a `json` report here too, for a
      // browser half to merge against; that half is gone and nothing reads it.
      reporter: ["text-summary", "lcov"],
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
        // Storybook fixtures back the component workshop and the Storybook
        // suite's stories; neither is the jsdom suite's subject, and the
        // Storybook suite carries no coverage instrumentation of its own —
        // see the comment on `coverage-ui` in mise-tasks.toml for why.
        "src/storybook/**",
        "src/**/*.stories.{ts,tsx}",
      ],
    },
    projects: [
      {
        extends: true,
        test: {
          name: "unit",
          environment: "jsdom",
          // Vitest would otherwise collect e2e/*.spec.ts, which is Playwright's
          // suite and needs a browser. The unit suites stay a jsdom-only run
          // that a contributor with no browser installed can still pass.
          include: ["src/**/*.test.{ts,tsx}"],
          setupFiles: ["./src/test/setup.ts"],
          css: false,
          restoreMocks: true,
        },
      },
      {
        extends: true,
        plugins: [storybookTest({ configDir: path.join(dirname, ".storybook") })],
        test: {
          name: "storybook",
          // Every story, played and checked for markup and interaction in a real
          // browser rather than jsdom's approximation.
          browser: {
            enabled: true,
            headless: true,
            provider: playwright({}),
            instances: [{ browser: "chromium" }],
          },
        },
      },
    ],
  },
});
