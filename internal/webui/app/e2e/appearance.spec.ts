/**
 * How the page looks: the colour scheme it follows, and a viewport narrow enough
 * to make it choose.
 *
 * Both are decisions the UI makes in CSS and in the basemap it loads, and neither
 * survives a jsdom test: `prefers-color-scheme` there is whatever the harness
 * stubs, and a layout with no layout engine behind it has no columns to count.
 */

import type { Page } from "@playwright/test";
import {
  expect,
  installOfflineBasemap,
  mapRegion,
  openLibrary,
  openStage,
  pinRendering,
  settleMap,
  test,
} from "./fixtures";

const LOOP_STAGE = { routeId: 4102, stageOrder: 1 };

/** The light palette, from the custom properties in index.css. */
const LIGHT_SURFACE = "rgb(255, 255, 255)";
/** The dark one, which the same file switches to at the media query. */
const DARK_SURFACE = "rgb(22, 21, 20)";

function backgroundOfBody(page: Page): Promise<string> {
  return page.evaluate(() => getComputedStyle(document.body).backgroundColor);
}

test.describe("in a light colour scheme", () => {
  test.use({ colorScheme: "light" });

  test("the page and the basemap are both the light ones", async ({
    offlinePage: page,
    basemapRequests,
  }) => {
    await openStage(page, LOOP_STAGE.routeId, LOOP_STAGE.stageOrder);

    expect(await backgroundOfBody(page)).toBe(LIGHT_SURFACE);
    expect(basemapRequests.length).toBeGreaterThan(0);
    expect(basemapRequests.every((url) => !url.includes("dark"))).toBe(true);
  });
});

test.describe("in a dark colour scheme", () => {
  test.use({ colorScheme: "dark" });

  test("the page and the basemap are both the dark ones", async ({
    offlinePage: page,
    basemapRequests,
  }) => {
    await openStage(page, LOOP_STAGE.routeId, LOOP_STAGE.stageOrder);

    expect(await backgroundOfBody(page)).toBe(DARK_SURFACE);
    // The basemap cannot follow the media query in CSS: it is a document fetched
    // over the network, so the choice is made in JavaScript and the dark style is
    // requested instead. That request is the assertion.
    expect(basemapRequests.some((url) => url.includes("dark"))).toBe(true);
    // And the map really painted it: the two styles differ only in their
    // background colour, so a map that ignored the choice would look identical to
    // the light one.
    const dark = await settleMap(page);
    expect(dark.length).toBeGreaterThan(0);
  });
});

test("the two schemes do not render the same map", async ({ browser, baseURL }) => {
  const served = baseURL ?? "";
  const shots: Buffer[] = [];
  // Two contexts rather than two tests, because the comparison is the assertion:
  // the same stage, the same camera and the same pinned rendering, differing only
  // in the scheme, so anything that differs in the image was drawn because of it.
  for (const colorScheme of ["light", "dark"] as const) {
    const context = await browser.newContext({ colorScheme, reducedMotion: "reduce" });
    const page = await context.newPage();
    const leaks = await installOfflineBasemap(page, served);
    await pinRendering(page);

    await page.goto(`${served}/routes/${LOOP_STAGE.routeId}/${LOOP_STAGE.stageOrder}`);
    shots.push(await settleMap(page));

    expect(leaks, "no request left the page for a third-party server").toEqual([]);
    await context.close();
  }

  const [light, dark] = shots;
  expect(light && dark && light.equals(dark)).toBe(false);
});

test.describe("on a narrow viewport", () => {
  test.use({ viewport: { width: 375, height: 667 } });

  test("the library stacks into one column", async ({ offlinePage: page }) => {
    await openLibrary(page);

    const cards = page.locator(".route-card");
    const count = await cards.count();
    expect(count).toBeGreaterThan(1);
    const first = await cards.nth(0).boundingBox();
    const second = await cards.nth(1).boundingBox();
    expect(first).not.toBeNull();
    expect(second).not.toBeNull();
    if (!first || !second) {
      return;
    }
    // One column: the second card sits below the first rather than beside it, and
    // neither of them overflows the viewport.
    expect(second.y).toBeGreaterThan(first.y + first.height - 1);
    expect(first.x + first.width).toBeLessThanOrEqual(375);
  });

  test("a stage still shows its map and its chart", async ({ offlinePage: page }) => {
    await openStage(page, LOOP_STAGE.routeId, LOOP_STAGE.stageOrder);

    const box = await mapRegion(page).boundingBox();
    expect(box).not.toBeNull();
    if (!box) {
      return;
    }
    expect(box.width).toBeLessThanOrEqual(375);
    expect(box.height).toBeGreaterThan(120);
    await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();
    await expect(page.getByRole("slider")).toBeVisible();
  });
});
