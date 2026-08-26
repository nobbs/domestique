/**
 * The browser suite's harness.
 *
 * Everything a whole-page test needs that a unit test does not: an offline
 * basemap, a page whose rendering does not drift between runs, and a way to wait
 * for a WebGL map to stop moving before asserting anything about it.
 *
 * The suite runs against `mise run demo` — the anonymous synthetic library in
 * `internal/demo`, served by `dev/demoapi` behind the production identity gate.
 * No test here knows a real route, and none can reach a provider: the demo has
 * nowhere to reach, and the one origin outside it the application would otherwise
 * ask for — the basemap style — is answered from `basemap.ts`. Anything else
 * leaving the page fails the test that let it out.
 */

import { expect, type Locator, type Page, test as playwrightTest } from "@playwright/test";
import {
  darkBasemapStyle,
  lightBasemapStyle,
  SECOND_BASEMAP_NAME,
  secondBasemapStyle,
  secondBasemapStyleUrl,
} from "./basemap";

/**
 * Which style documents the service tells the browser to load.
 *
 * Read from the running API rather than hard-coded, so the suite intercepts
 * whatever a deployment is configured with instead of asserting the default and
 * quietly going to the network when somebody changes it.
 */
interface StyleUrls {
  light: string;
  dark: string | undefined;
  /** Every configured style, which is what the chooser's previews load. */
  others: Set<string>;
}

/**
 * Rendering that a screenshot can be compared against itself.
 *
 * Animations and transitions are already suppressed by Playwright's
 * reduced-motion setting for anything that honours the media query; this turns
 * them off for everything else as well. The font stack collapses to the
 * platform's generic families, so text metrics do not depend on which fonts the
 * machine running the suite happens to have installed. `caret-color` matters
 * because a blinking caret is a pixel that changes on its own.
 */
const DETERMINISTIC_RENDERING = `
  *, *::before, *::after {
    animation-delay: -0.0001ms !important;
    animation-duration: 0.0001ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.0001ms !important;
    transition-delay: 0s !important;
    caret-color: transparent !important;
    scroll-behavior: auto !important;
  }
  :root, body, button, input, select, textarea {
    font-family: sans-serif !important;
  }
  code, kbd, pre, samp {
    font-family: monospace !important;
  }
`;

async function styleUrls(
  page: Page,
  baseUrl: string,
  headers: Record<string, string>,
): Promise<StyleUrls> {
  const response = await page.request.get(`${baseUrl}/v1/webui/config`, { headers });
  expect(response.ok(), "the demo API serves its browser configuration").toBeTruthy();
  const payload = (await response.json()) as {
    basemaps?: { styleUrl?: string; styleUrlDark?: string }[];
  };
  // The first entry is the one a browser that has chosen nothing loads, which is
  // every browser this harness starts.
  const first = payload.basemaps?.[0];
  expect(first?.styleUrl, "the service names a basemap style").toBeTruthy();

  /*
   * Every other configured entry, because the chooser previews them: opening it
   * loads each basemap's style document to draw its thumbnail. Unstubbed, those
   * would reach the public providers and be reported as leaks — which is a real
   * thing this harness is for, and not what these tests are about.
   */
  const others = new Set<string>();
  for (const basemap of payload.basemaps ?? []) {
    for (const url of [basemap.styleUrl, basemap.styleUrlDark]) {
      if (url !== undefined && url !== "") {
        others.add(url);
      }
    }
  }

  return { light: first?.styleUrl ?? "", dark: first?.styleUrlDark, others };
}

/** What an offline page may be given beyond the defaults. */
export interface OfflineOptions {
  /**
   * Collects the style documents the page really asked for, in order.
   *
   * The colour scheme decides which one the application loads, and a canvas
   * cannot be read back from JavaScript — MapLibre keeps no drawing buffer — so
   * this is how a test knows the dark basemap was chosen rather than merely that
   * the page looks different.
   */
  requested?: string[];
  /**
   * Headers added to every same-origin request.
   *
   * The Vite dev server injects the identity assertion and the configured origin
   * on its way through, so a page talking to it needs none. A page talking
   * straight to the Go service does: the gate is the production one, and a
   * browser holds no credential. Injecting them here is the same arrangement as
   * the dev proxy's, in the place a browser test can put it.
   *
   * `Origin` is the exception. It is browser-managed: Chromium keeps the page's
   * own origin on a request whatever a route handler asks for, so a request that
   * carries one has to be forwarded by the harness rather than merely annotated.
   * That is the hop the dev proxy is, and it is only a hop — the request is
   * still the one the shipped client composed, and the answer is still the one
   * the shipped client parses.
   */
  headers?: Record<string, string>;
  /**
   * Offers an isolated extra basemap by rewriting the configuration the page
   * reads. The demo's cartographies are public styles, which this hermetic suite
   * must never fetch; this entry lets the chooser test prove its request and
   * attribution behaviour from memory instead.
   */
  secondBasemap?: boolean;
}

/**
 * Cuts a page off from everything but the service in front of it.
 *
 * The two style URLs the service names are answered from memory; every other
 * cross-origin request is refused and recorded in the returned list, so a caller
 * can fail at teardown rather than at the moment of the request — a leak is then
 * reported as itself instead of as whatever the aborted request broke.
 */
export async function installOfflineBasemap(
  page: Page,
  baseUrl: string,
  options: OfflineOptions = {},
): Promise<string[]> {
  const requested = options.requested ?? [];
  // Header names are case-insensitive on the wire and Playwright reports a
  // request's own in lower case, so a caller's spelling must not decide whether
  // an override replaces a header or arrives beside it under a second name.
  const headers = Object.fromEntries(
    Object.entries(options.headers ?? {}).map(([name, value]) => [name.toLowerCase(), value]),
  );
  const origin = new URL(baseUrl).origin;
  const styles = await styleUrls(page, baseUrl, headers);
  const secondStyle = options.secondBasemap ? secondBasemapStyleUrl(styles.light) : null;
  const leaks: string[] = [];

  await page.route("**/*", async (route) => {
    const url = route.request().url();
    if (secondStyle !== null && url === `${origin}/v1/webui/config`) {
      const answered = await route.fetch({ headers: { ...route.request().headers(), ...headers } });
      const payload = (await answered.json()) as { basemaps: unknown[] };
      // Appended rather than substituted, so the first entry stays the one every
      // other test loads and the added one is what a press has to reach for.
      payload.basemaps.push({
        name: SECOND_BASEMAP_NAME,
        styleUrl: secondStyle,
        darkCartography: true,
      });
      await route.fulfill({ contentType: "application/json", body: JSON.stringify(payload) });

      return;
    }
    if (url === secondStyle) {
      requested.push(url);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(secondBasemapStyle),
      });

      return;
    }
    if (url.startsWith(origin) || url.startsWith("data:") || url.startsWith("blob:")) {
      const forwarded = { ...route.request().headers(), ...headers };
      // A browser attaches Origin to everything that is not a GET or a HEAD, and
      // it attaches its own. Requests that have to arrive with a different one
      // are made from outside the browser and their answer handed back to it.
      if (headers.origin !== undefined && "origin" in route.request().headers()) {
        await route.fulfill({ response: await route.fetch({ headers: forwarded }) });

        return;
      }
      await route.continue({ headers: forwarded });

      return;
    }
    if (url === styles.light || url === styles.dark) {
      requested.push(url);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(url === styles.dark ? darkBasemapStyle : lightBasemapStyle),
      });

      return;
    }
    // A configured basemap the map itself never loaded: the chooser's preview
    // asked for it. Answered offline like the rest, and deliberately not
    // recorded — `requested` is how a test knows which ground the map went and
    // fetched, and a thumbnail is not the map.
    if (styles.others.has(url)) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(lightBasemapStyle),
      });

      return;
    }
    leaks.push(url);
    await route.abort("blockedbyclient");
  });

  return leaks;
}

/** Pins the rendering of a page, before anything in it has run. */
export async function pinRendering(page: Page): Promise<void> {
  await page.addInitScript((css: string) => {
    const apply = () => {
      const element = document.createElement("style");
      element.setAttribute("data-test-rendering", "deterministic");
      element.textContent = css;
      document.head.append(element);
    };
    if (document.head) {
      apply();
    } else {
      document.addEventListener("DOMContentLoaded", apply, { once: true });
    }
  }, DETERMINISTIC_RENDERING);
}

/**
 * A page that cannot reach anything but the service in front of it, and whose
 * rendering does not drift between runs.
 */
export const test = playwrightTest.extend<{
  offlinePage: Page;
  basemapRequests: string[];
}>({
  /** The style documents the page asked for, in order. */
  // Playwright reads a fixture's dependencies out of this destructuring, and this one has none.
  // biome-ignore lint/correctness/noEmptyPattern: see above
  basemapRequests: async ({}, use) => {
    await use([]);
  },

  offlinePage: async ({ page, baseURL, basemapRequests }, use) => {
    const leaks = await installOfflineBasemap(page, baseURL ?? "", {
      requested: basemapRequests,
    });
    await pinRendering(page);

    await use(page);

    expect(leaks, "no request left the page for a third-party server").toEqual([]);
  },
});

export { expect };

/** The entry page, once the library has arrived and the map is drawn. */
export async function openLibrary(page: Page): Promise<void> {
  await page.goto("/");
  await settleMap(page);
}

/** Opens the compact workspace when the map is being viewed on a narrow screen. */
export async function openWorkspace(page: Page): Promise<void> {
  const browse = page.getByRole("button", { name: "Browse routes" });
  if (await browse.isVisible()) {
    await browse.click();
  }
}

/** Opens the library search and returns the field it puts under the control. */
export async function openSearch(page: Page): Promise<Locator> {
  const field = page.getByRole("searchbox", { name: "Search the route library" });
  await openWorkspace(page);
  if (!(await field.isVisible())) {
    await page.getByRole("button", { name: "Search the route library" }).click();
  }
  await expect(field).toBeVisible();

  return field;
}

/**
 * One route, opened over the library map.
 *
 * The route is a panel rather than a page, so it is addressed by the query the
 * panel carries. Going there directly is what a shared link does, and it is the
 * shortest way into the state every test in this suite starts from.
 */
export async function openRoute(
  page: Page,
  provider: string,
  routeId: number,
  stageOrder: number,
): Promise<void> {
  await page.goto(`/?route=${provider}%2F${routeId}%2F${stageOrder}`);
  await openWorkspace(page);
  await expect(page.getByRole("button", { name: /^Search \d+ routes?$/ })).toBeVisible();
  await settleMap(page);
}

/** The sync page, once the service has answered what it is doing. */
export async function openSync(page: Page): Promise<void> {
  await page.goto("/sync");
  await expect(page.getByRole("heading", { level: 1, name: "Sync" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Now" })).toBeVisible();
}

/** The map's own element, which carries the map's accessible name. */
export function mapRegion(page: Page): Locator {
  return page.locator(".route-map");
}

/**
 * Waits until the map has stopped changing.
 *
 * MapLibre paints asynchronously and eases into its initial bounds, so there is
 * no single event after which the canvas is final — `load` fires while the
 * camera is still moving. Screenshotting until two consecutive frames match is
 * the honest way to wait for "settled", and it is what makes the comparisons in
 * this suite reproducible rather than a race against an animation.
 *
 * Returns the settled image, so a caller that wants to compare two states does
 * not have to take a third screenshot to get one.
 */
export async function settleMap(page: Page, attempts = 20): Promise<Buffer> {
  const region = mapRegion(page);
  await expect(region).toBeVisible();
  await expect(page.locator(".maplibregl-canvas")).toBeVisible();

  let previous = await region.screenshot();
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    await page.waitForTimeout(250);
    const current = await region.screenshot();
    if (current.equals(previous)) {
      return current;
    }
    previous = current;
  }

  throw new Error("the map never stopped repainting");
}

/** The elevation chart's scrubber, which is also its keyboard control. */
export function profileScrubber(page: Page): Locator {
  return page.getByRole("slider");
}
