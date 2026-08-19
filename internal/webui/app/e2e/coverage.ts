/**
 * V8 coverage collection for the browser suite.
 *
 * The suite exists because the map and the page-level components cannot be
 * observed in jsdom, and until this file existed none of what it reached entered
 * the number Codecov reads: `test:coverage` is Vitest alone, so those components
 * reported low however thoroughly Chromium had just driven them.
 *
 * Chromium reports coverage over CDP in V8's own format, which is the format
 * Vitest's `v8` provider already consumes. That is what makes the two halves
 * mergeable rather than merely comparable — both describe statements the same
 * way, so a statement reached by both is one statement, not two.
 *
 * Only the `dev-server` project is collected, and this file is imported by
 * `e2e/fixtures.ts` alone, which is that project's harness. The `bundle` project
 * has its own fixtures and is deliberately not measured: it drives `vite build`
 * output, `build.sourcemap` is unset, so its V8 ranges cannot be attributed back
 * to `src/**` without shipping production source maps purely for measurement —
 * and its specs assert the serving contract rather than component behaviour, so
 * what it would add to a `src/**` number is close to nothing for a real cost.
 *
 * Collection stops at the raw output. Turning it into a report is
 * `scripts/coverage.ts`, which runs once the suite has finished and can
 * attribute against the whole tree rather than against one test's modules.
 */

import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import type { Page } from "@playwright/test";

/**
 * Where raw V8 output is written.
 *
 * Unset means the suite is not being measured, which is the normal case:
 * `make ui-browser-test` runs it as a test suite and pays nothing for coverage.
 * `scripts/coverage.ts` sets this when it runs the suite for its numbers.
 *
 * Deliberately not `DOMESTIQUE_`-prefixed. The demo API the suite runs against
 * inherits this environment and treats every variable in that namespace as one
 * of its settings, refusing the ones it does not know, so a prefixed name here
 * would stop the stack from starting at all.
 */
const directory = process.env.WEBUI_COVERAGE_DIR ?? "";

/** Whether this run is being measured at all. */
export const measuringCoverage = directory !== "";

/**
 * Distinguishes the files of one run from each other.
 *
 * A test id is unique within a run, but a test may open more than one page and
 * each page reports its own coverage, so the id alone would have one page
 * overwrite another's.
 */
let sequence = 0;

/** Begins recording, before the page has loaded anything. */
export async function startCoverage(page: Page): Promise<void> {
  await page.coverage.startJSCoverage({
    // Coverage is wanted for the whole test, and every one of these navigates at
    // least once. Resetting on navigation would report only the last page.
    resetOnNavigation: false,
    // Inline and evaluated scripts have no source map back to `src/**`, so they
    // could not be attributed even if they were collected.
    reportAnonymousScripts: false,
  });
}

/**
 * Stops recording and writes what this page reached.
 *
 * Only the application's own modules are kept. The dev server also serves
 * dependencies, its own client, and the CSS it turns into JavaScript, none of
 * which the `ui` flag is about; dropping them here keeps the raw files small
 * enough to read when the numbers look wrong. Which of the remaining files are
 * actually measured is decided in `scripts/coverage.ts`, against the same two
 * lists Vitest uses, so that judgement is made once rather than twice.
 */
export async function stopCoverage(page: Page, testId: string): Promise<void> {
  const entries = await page.coverage.stopJSCoverage();
  const application = entries
    .filter((entry) => new URL(entry.url).pathname.startsWith("/src/"))
    .map((entry) => ({ url: entry.url, source: entry.source, functions: entry.functions }));

  if (application.length === 0) {
    return;
  }

  sequence += 1;
  await mkdir(directory, { recursive: true });
  await writeFile(
    path.join(directory, `${testId}-${sequence}.json`),
    JSON.stringify(application),
    "utf8",
  );
}
