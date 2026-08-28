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

// `dev/demo.sh --tailnet` fronts the dev server with `tailscale serve`, which
// takes a connection from the tailnet and forwards it to a loopback address.
// Vite stays on loopback either way — the tailnet is reached by what forwards
// to it, not by widening what it listens on — and the port is the same on both
// sides, so hot reload needs nothing.
//
// Two things do change. The Host header arrives as this machine's MagicDNS
// name, and Vite serves only Hosts it trusts — localhost and IP literals on its
// own, and a MagicDNS name is neither; so the tailnet suffix is named here, the
// name in front of it being whichever tailnet this machine joined, which a
// checked-in file cannot know. And `localhost` resolves to ::1 first, which is
// the only address Vite then listens on, while Serve forwards to 127.0.0.1 and
// gets nothing: the two are named the same and are not the same socket. So the
// address is spelled out for as long as something is forwarding to it.
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
    // One level below the embedded root. `emptyOutDir` clears whatever it is
    // pointed at, and the directory Go embeds holds a committed placeholder
    // that keeps the embed pattern valid before the first build — so the two
    // cannot be the same directory without the bundler deleting a tracked file
    // on every run.
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
    // The terminal summary a contributor reads, and the JUnit report CI
    // uploads to Codecov's test analytics beside the other suite's own.
    // Written on every run rather than behind a flag, so the file a failure
    // is read from locally is the same one the pull request comment was
    // assembled from.
    //
    // `reporters`/`outputFile` are root-level config: Vitest applies them to
    // the whole invocation, not per project, so filtering with `--project`
    // still writes here. VITEST_JUNIT_SUFFIX is what keeps `mise run ui-test`
    // and `ui-storybook-test` from clobbering each other's report — see the
    // scripts in package.json that set it.
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
          // Every story, played and checked for markup and interaction in a
          // real browser rather than jsdom's DOM
          // approximation — see the comment above BasemapPicker.stories.tsx
          // and its siblings for why some component suites live here instead
          // of in the jsdom project.
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
