/**
 * The harness for the `bundle` and `mutations` projects.
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
 * the session cookie, and a state-changing one carries the configured browser
 * origin. The gate itself is untouched — it is the production gate, reading a
 * real session row — and the token is the throwaway one `dev/demoapi` mints for
 * itself at start-up. What the gate refuses is asserted directly, by a request
 * that presents the wrong origin.
 */

import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { expect, type Page, test as playwrightTest } from "@playwright/test";
import { installOfflineBasemap, pinRendering } from "../fixtures";

/**
 * Where `dev/demo.sh` leaves the session it minted.
 *
 * Read from disk rather than passed in, because the demo mints it at start-up:
 * the suite's own web server is what produced the file, it holds a token for a
 * row in that process's own database, and it is deleted when the demo stops.
 * Resolved against this file rather than the working directory, so it does not
 * depend on where the suite was started from.
 */
const SESSION_FILE =
  process.env.DOMESTIQUE_DEMO_SESSION_FILE ??
  fileURLToPath(new URL("../../../../../.local/demo/session", import.meta.url));

/** The origin `dev/demo.sh` configures the service to serve its UI at. */
const BROWSER_ORIGIN = process.env.DOMESTIQUE_DEV_ORIGIN ?? "https://127.0.0.1:9";

/** The cookie the sign-in flow issues and the gate reads. */
const SESSION_COOKIE = "__Host-domestique_session";

async function sessionToken(): Promise<string> {
  const token = (await readFile(SESSION_FILE, "utf8")).trim();
  expect(token, `the demo minted a session into ${SESSION_FILE}`).not.toBe("");

  return token;
}

async function identityHeaders(): Promise<Record<string, string>> {
  return {
    cookie: `${SESSION_COOKIE}=${await sessionToken()}`,
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
    // A Cookie header set on the route is dropped by the browser, so the session
    // goes into the jar. By domain rather than url: a url of http would force
    // secure off, and the __Host- prefix needs it on; 127.0.0.1 is a secure context.
    await page.context().addCookies([
      {
        name: SESSION_COOKIE,
        value: await sessionToken(),
        domain: new URL(served).hostname,
        path: "/",
        secure: true,
        httpOnly: true,
        sameSite: "Lax",
      },
    ]);
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
