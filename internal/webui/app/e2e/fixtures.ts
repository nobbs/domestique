/**
 * The browser suite's harness.
 *
 * Everything a whole-page test needs that a unit test does not: an offline
 * basemap, a page whose rendering does not drift between runs, and a way to wait
 * for a WebGL map to stop moving before asserting anything about it.
 *
 * The suite runs against `make demo` — the anonymous synthetic library in
 * `internal/demo`, served by `dev/demoapi` behind the production identity gate.
 * No test here knows a real route, and none can reach a provider: the demo has
 * nowhere to reach, and the one origin outside it the application would otherwise
 * ask for — the basemap style — is answered from `basemap.ts`. Anything else
 * leaving the page fails the test that let it out.
 */

import { expect, type Locator, type Page, test as playwrightTest } from "@playwright/test";
import { darkBasemapStyle, lightBasemapStyle } from "./basemap";

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

async function styleUrls(page: Page, baseUrl: string): Promise<StyleUrls> {
  const response = await page.request.get(`${baseUrl}/v1/webui/config`);
  expect(response.ok(), "the demo API serves its browser configuration").toBeTruthy();
  const payload = (await response.json()) as {
    tile_style_url?: string;
    tile_style_url_dark?: string;
  };
  expect(payload.tile_style_url, "the service names a basemap style").toBeTruthy();

  return { light: payload.tile_style_url ?? "", dark: payload.tile_style_url_dark };
}

/**
 * Cuts a page off from everything but the service in front of it.
 *
 * The two style URLs the service names are answered from memory; every other
 * cross-origin request is refused and recorded in the returned list, so a caller
 * can fail at teardown rather than at the moment of the request — a leak is then
 * reported as itself instead of as whatever the aborted request broke.
 *
 * `requested` collects the style documents the page really asked for, in order.
 * The colour scheme decides which one the application loads, and a canvas cannot
 * be read back from JavaScript — MapLibre keeps no drawing buffer — so this is how
 * a test knows the dark basemap was chosen rather than merely that the page looks
 * different.
 */
export async function installOfflineBasemap(
  page: Page,
  baseUrl: string,
  requested: string[] = [],
): Promise<string[]> {
  const origin = new URL(baseUrl).origin;
  const styles = await styleUrls(page, baseUrl);
  const leaks: string[] = [];

  await page.route("**/*", async (route) => {
    const url = route.request().url();
    if (url.startsWith(origin) || url.startsWith("data:") || url.startsWith("blob:")) {
      await route.continue();

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
    const leaks = await installOfflineBasemap(page, baseURL ?? "", basemapRequests);
    await pinRendering(page);

    await use(page);

    expect(leaks, "no request left the page for a third-party server").toEqual([]);
  },
});

export { expect };

/** The library page, once it has stages to show. */
export async function openLibrary(page: Page): Promise<void> {
  await page.goto("/");
  await expect(page.getByRole("link", { name: /Valley floor/ }).first()).toBeVisible();
}

/** One stage's detail page, once its map and profile are on screen. */
export async function openStage(page: Page, routeId: number, stageOrder: number): Promise<void> {
  await page.goto(`/routes/${routeId}/${stageOrder}`);
  await settleMap(page);
}

/** The map's own element, which carries the accessible name of the stage. */
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
