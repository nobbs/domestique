import { defineConfig, devices, type ReporterDescription } from "@playwright/test";

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
 * That one stack serves the suite three times over, as three projects:
 *
 *   dev-server  the specs in `e2e`, against the Vite dev server. The UI as it is
 *               being written, which is what a change to it should be checked as.
 *   bundle      `e2e/contract/served-bundle.spec.ts`, against the Go service
 *               directly: the production bundle from `internal/webui`'s embed
 *               handler, the real routes behind it, and the identity gate, cache
 *               headers and content security policy a deployment applies. A
 *               handler test and a parser test can both pass while the JSON they
 *               assume has drifted apart, and this is the project that reads a
 *               real response with the real client.
 *   mutations   `e2e/contract/mutations.spec.ts`, against that same service. Its
 *               own project only because of what it does rather than what it
 *               drives: see the `dependencies` on it below.
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
 * What narrates the run: `list` in a terminal, `github` on a runner.
 *
 * `github` is the only built-in reporter that emits `::error` workflow commands,
 * which GitHub renders as inline annotations on the pull request's Files-changed
 * view, at the file and line the assertion failed on. It narrates the run as
 * `list` does besides, so nothing is lost in the Actions log by swapping one for
 * the other. The documented caveat — that a matrix strategy multiplies each
 * annotation by the number of legs — does not apply: this suite runs under no
 * matrix, and `fullyParallel: false` keeps one file's tests on one worker.
 */
const progressReporter: ReporterDescription = process.env.CI ? ["github"] : ["list"];

export default defineConfig({
  testDir: "./e2e",
  // Two workers, over one demo API, one database and one dev server.
  //
  // The cost here is the browser — a software WebGL renderer painting a map for
  // every test — and that is CPU the runner has spare: this suite is the last
  // thing left running in the UI job, alone on four vCPUs, and measured at
  // around 1.6 of them on a single worker. What stopped it being more than one
  // was never the browser but the shared stack underneath it, and the projects
  // below are what make that safe rather than this number.
  //
  // `fullyParallel: false` is what keeps the two honest: a worker takes a whole
  // file, so the order within one is still the order it is written in, and only
  // separate files ever run at the same time.
  workers: 2,
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
  // The JUnit report goes beside the Vitest one rather than under .playwright/,
  // which holds what a failed run leaves for a human to look at. CI uploads both
  // under the `ui` flag; see the test-results block in .github/workflows/ci.yml.
  reporter: [
    progressReporter,
    ["html", { outputFolder: "../../../.playwright/report", open: "never" }],
    ["junit", { outputFile: "../../../.test-results/ui/browser.xml" }],
  ],
  // Two read-only projects that may overlap, and one that may not.
  //
  // Everything above reads: it opens pages, drives a map, scrubs a chart and
  // follows links, and two workers doing that at once over one service is only
  // two readers. `mutations` is the exception — it toggles the schedule, and its
  // "run now" re-seeds the whole synthetic library — so it is split out of
  // `e2e/contract` into a project of its own and made to wait for both of the
  // others. That is the whole of what keeps `workers: 2` safe: a re-seed can
  // never land under a test that is reading what it rewrites.
  //
  // It is one file, and `fullyParallel: false` holds one file to one worker, so
  // the mutations still run one after another as they are written.
  projects: [
    {
      name: "dev-server",
      testDir: "./e2e",
      // The contract projects' specs live below this directory and must not be
      // collected twice, once against a server they were not written for.
      testIgnore: "**/contract/**",
      use: { baseURL: DEV_SERVER_URL },
    },
    {
      name: "bundle",
      testDir: "./e2e/contract",
      testIgnore: "**/mutations.spec.ts",
      use: { baseURL: SERVICE_URL },
    },
    {
      name: "mutations",
      testDir: "./e2e/contract",
      testMatch: "**/mutations.spec.ts",
      dependencies: ["dev-server", "bundle"],
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
    // previous build left embedded.
    command: "./dev/demo.sh --with-bundle",
    cwd: "../../..",
    // Through the dev server's proxy rather than at its root, because the root
    // is served before the proxy behind it can carry a request: `dev/demo.sh`
    // starts Vite last and only once the API is answering, but Vite answers for
    // itself while it is still optimising its dependencies, and the suite's very
    // first act in every test is a proxied `/v1/webui/config`. Waiting on a path
    // that has to travel the whole way is what makes "ready" mean ready — at any
    // number of workers, and most visibly at more than one, where two tests ask
    // at once and a root-only check let them both ask too early.
    url: `${DEV_SERVER_URL}/healthz`,
    // Locally, a demo already running is the one to test against; in CI there is
    // never one to reuse, and silently using a stale server would be worse than
    // failing to start.
    reuseExistingServer: !process.env.CI,
    // A cold run bundles the UI and compiles the demo API: the budget is the Vite
    // production build plus the Go build plus the dev server's first transform.
    timeout: 300_000,
    stdout: "pipe",
    stderr: "pipe",
  },
});
