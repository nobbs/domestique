import { defineConfig, devices } from "@playwright/test";

/**
 * The browser suite: the whole page, in a real browser, over the synthetic demo
 * library.
 *
 * It exists because two things the UI is mostly made of cannot be observed by the
 * Vitest suites — MapLibre needs WebGL, and the interactions that matter here
 * (scrubbing a chart, dragging a stretch out of a map, following a link into a
 * route) span components rather than living inside one. This is not a second home
 * for logic tests: anything a unit test can reach belongs there, where it runs in
 * a second and needs no browser.
 *
 * `webServer` is `mise run demo`'s own script, so the suite runs against exactly the
 * stack a developer would start by hand: `dev/demoapi` serving the invented
 * library in `internal/demo` behind the production identity gate, with the Vite
 * dev server in front of it. Nothing here reads a real route, and the fixtures in
 * `e2e/fixtures.ts` answer the only third-party request the application makes.
 *
 * That one stack serves the suite twice over, as two projects:
 *
 *   dev-server  the specs in `e2e`, against the Vite dev server. The UI as it is
 *               being written, which is what a change to it should be checked as.
 *   bundle      the specs in `e2e/contract`, against the Go service directly: the
 *               production bundle from `internal/webui`'s embed handler, the real
 *               routes behind it, and the identity gate, cache headers and content
 *               security policy a deployment applies. A handler test and a parser
 *               test can both pass while the JSON they assume has drifted apart,
 *               and this is the project that reads a real response with the real
 *               client.
 *
 * Everything about the environment that a rendered pixel depends on is pinned
 * below — viewport, scale factor, colour scheme, locale, time zone and motion —
 * because a page that renders differently between two runs cannot be asserted
 * about at all. No screenshot is stored in the repository: the suite compares a
 * page against itself within a run, and writes images, traces and a report under
 * `.playwright/` only when something failed.
 */
/** Where the Vite dev server answers, as `dev/demo.sh` starts it. */
const DEV_SERVER_URL = "http://localhost:5173";

/**
 * Where the Go service answers, which is also where it serves the bundle.
 *
 * `dev/demo.sh` takes the port from the same variable, so overriding it moves
 * both and the two stay in step.
 */
const SERVICE_URL = `http://127.0.0.1:${process.env.DOMESTIQUE_DEMO_PORT ?? "8082"}`;

/**
 * Whether this run is being measured, which `scripts/coverage.ts` decides.
 *
 * A measured run is the `dev-server` project alone — the `bundle` project drives
 * minified build output that cannot be attributed back to `src/**` — so it does
 * not need the production bundle the demo would otherwise build first.
 */
const measuringCoverage = process.env.WEBUI_COVERAGE_DIR !== undefined;

export default defineConfig({
  testDir: "./e2e",
  // One demo API, one database, one dev server, and a software WebGL renderer for
  // every map: the suite is serial on purpose. Its cost is the browser, not the
  // number of workers.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  // A flaky browser test that passes on the second attempt is a browser test that
  // reports nothing. Failures here are meant to be reproducible.
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  // Gitignored, and at the repository root beside the coverage reports, so that
  // one place holds everything a run leaves behind for a human to look at.
  outputDir: "../../../.playwright/results",
  reporter: [["list"], ["html", { outputFolder: "../../../.playwright/report", open: "never" }]],
  projects: [
    {
      name: "dev-server",
      testDir: "./e2e",
      // The bundle project's specs live below this directory and must not be
      // collected twice, once against a server they were not written for.
      testIgnore: "**/contract/**",
      use: { baseURL: DEV_SERVER_URL },
    },
    {
      name: "bundle",
      testDir: "./e2e/contract",
      use: { baseURL: SERVICE_URL },
    },
  ],
  use: {
    ...devices["Desktop Chrome"],
    viewport: { width: 1280, height: 900 },
    deviceScaleFactor: 1,
    colorScheme: "light",
    // Not a top-level test option in this version, so it goes through the context
    // options the fixtures' pages are built from.
    contextOptions: { reducedMotion: "reduce" },
    locale: "en-GB",
    timezoneId: "UTC",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
    launchOptions: {
      // MapLibre needs a WebGL context, and a headless container has no GPU:
      // ANGLE's software rasteriser provides one. Hinting is disabled because
      // hinted glyphs differ between machines, which is exactly the kind of
      // difference a comparison here must not pick up.
      args: [
        "--use-gl=angle",
        "--use-angle=swiftshader",
        "--enable-unsafe-swiftshader",
        "--font-render-hinting=none",
      ],
    },
  },
  webServer: {
    // --with-bundle builds the browser UI before the demo API is compiled, so the
    // bundle the bundle project drives is the current one rather than whatever a
    // previous build left embedded. A coverage run drives only the dev server, so
    // it skips a production build it would never look at.
    command: measuringCoverage ? "./dev/demo.sh" : "./dev/demo.sh --with-bundle",
    cwd: "../../..",
    // The dev server is the last thing `dev/demo.sh` starts, and it starts it only
    // after the API answers its own health check, so waiting here waits for both.
    url: DEV_SERVER_URL,
    // Locally, a demo already running is the one to test against; in CI there is
    // never one to reuse, and silently using a stale server would be worse than
    // failing to start.
    reuseExistingServer: !process.env.CI,
    // A cold run bundles the UI and compiles the demo API: the budget is the Vite
    // production build plus the Go build plus the dev server's first transform.
    timeout: 180_000,
    stdout: "pipe",
    stderr: "pipe",
  },
});
