/**
 * The harness for the bundle project.
 *
 * These tests talk to the Go service itself: the production bundle served by
 * `internal/webui`'s embed handler, the real handlers behind it, and a SQLite
 * database seeded with the synthetic library. Nothing between the browser and
 * the service rewrites anything, which is the point — a handler test and a
 * parser test can both pass while the JSON they each assume has drifted apart,
 * and only the real bundle reading a real response catches that.
 *
 * Two things the Vite dev server does on the way through have to be done here
 * instead, because there is no proxy in this arrangement: every request carries
 * the identity assertion, and a state-changing one carries the configured browser
 * origin. The gate itself is untouched — it is the production verifier, checking
 * a real signature, audience, expiry and address — and the assertion is the
 * throwaway one `dev/demoapi` mints for itself at start-up. What the gate refuses
 * is asserted directly, by a request that presents the wrong origin.
 */

import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { expect, type Page, test as playwrightTest } from "@playwright/test";
import { installOfflineBasemap, pinRendering } from "../fixtures";

/**
 * Where `dev/demo.sh` leaves the assertion it minted.
 *
 * Read from disk rather than passed in, because the demo mints it at start-up:
 * the suite's own web server is what produced the file, it holds a token signed
 * by a key that existed only for that process, and it is deleted when the demo
 * stops. Resolved against this file rather than the working directory, so it does
 * not depend on where the suite was started from.
 */
const ASSERTION_FILE =
  process.env.DOMESTIQUE_DEMO_ASSERTION_FILE ??
  fileURLToPath(new URL("../../../../../.local/demo/assertion", import.meta.url));

/** The origin `dev/demo.sh` configures the service to serve its UI at. */
const BROWSER_ORIGIN = process.env.DOMESTIQUE_DEV_ORIGIN ?? "https://127.0.0.1:9";

async function identityHeaders(): Promise<Record<string, string>> {
  const assertion = (await readFile(ASSERTION_FILE, "utf8")).trim();
  expect(assertion, `the demo minted an assertion into ${ASSERTION_FILE}`).not.toBe("");

  return {
    // The header Cloudflare Access sets and the service verifies. Lower-cased
    // because that is how a request carries it over HTTP/1.1 either way.
    "cf-access-jwt-assertion": assertion,
    // A state-changing request must come from the origin the service is
    // configured to serve at, which is not the loopback address this test reaches
    // it on. The dev proxy rewrites it for the same reason.
    origin: BROWSER_ORIGIN,
  };
}

/** One API request the page made, as the test can see it from outside. */
export interface ApiCall {
  method: string;
  path: string;
  status: number;
}

export const test = playwrightTest.extend<{
  /** A page served by the Go service, carrying an identity it can verify. */
  bundlePage: Page;
  /**
   * The headers a request needs to pass the gate, for a test that asks the
   * service something directly rather than through the page.
   */
  identity: Record<string, string>;
  /** Every `/v1` request the page made, in order, with the status it got. */
  apiCalls: ApiCall[];
}>({
  // biome-ignore lint/correctness/noEmptyPattern: this fixture has no dependencies
  apiCalls: async ({}, use) => {
    await use([]);
  },

  // biome-ignore lint/correctness/noEmptyPattern: this fixture has no dependencies
  identity: async ({}, use) => {
    await use(await identityHeaders());
  },

  bundlePage: async ({ page, baseURL, apiCalls, identity }, use) => {
    const served = baseURL ?? "";
    const leaks = await installOfflineBasemap(page, served, { headers: identity });
    await pinRendering(page);

    page.on("response", (response) => {
      const { pathname } = new URL(response.url());
      if (pathname.startsWith("/v1/")) {
        apiCalls.push({
          method: response.request().method(),
          path: pathname,
          status: response.status(),
        });
      }
    });

    await use(page);

    expect(leaks, "no request left the page for a third-party server").toEqual([]);
  },
});

export { expect };

/** The calls made to one endpoint, so a test can say what it expected of it. */
export function callsTo(calls: ApiCall[], method: string, path: string): ApiCall[] {
  return calls.filter((call) => call.method === method && call.path === path);
}
