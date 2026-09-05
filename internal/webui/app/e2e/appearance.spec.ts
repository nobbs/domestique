/**
 * How the page looks: the colour scheme it follows, and a viewport narrow enough
 * to make it choose.
 *
 * Both are decisions the UI makes in CSS and in the basemap it loads, and neither
 * survives a jsdom test: `prefers-color-scheme` there is whatever the harness
 * stubs, and a layout with no layout engine behind it has no columns to count.
 */

import type { Page } from "@playwright/test";
import { BASEMAP_ATTRIBUTION_TEXT } from "./basemap";
import {
  expect,
  installOfflineBasemap,
  mapRegion,
  openLibrary,
  openRoute,
  openSearch,
  openWorkspace,
  pinRendering,
  settleMap,
  test,
} from "./fixtures";

const LOOP_ROUTE = { provider: "veloplanner", sourceRouteId: 4102, stageOrder: 1 };

/** `--base` in the light palette, from the custom properties in index.css. */
const LIGHT_SURFACE = "rgb(243, 245, 246)";
/** The dark one, which the same file switches to at the media query. */
const DARK_SURFACE = "rgb(16, 19, 22)";

function backgroundOfBody(page: Page): Promise<string> {
  return page.evaluate(() => getComputedStyle(document.body).backgroundColor);
}

test.describe("in a light colour scheme", () => {
  test.use({ colorScheme: "light" });

  test("the page and the basemap are both the light ones", async ({
    offlinePage: page,
    basemapRequests,
  }) => {
    await openRoute(page, LOOP_ROUTE.provider, LOOP_ROUTE.sourceRouteId, LOOP_ROUTE.stageOrder);

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
    await openRoute(page, LOOP_ROUTE.provider, LOOP_ROUTE.sourceRouteId, LOOP_ROUTE.stageOrder);

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

    await page.goto(
      `${served}/?route=${LOOP_ROUTE.provider}%2F${LOOP_ROUTE.sourceRouteId}%2F${LOOP_ROUTE.stageOrder}`,
    );
    shots.push(await settleMap(page));

    expect(leaks, "no request left the page for a third-party server").toEqual([]);
    await context.close();
  }

  const [light, dark] = shots;
  expect(light && dark && light.equals(dark)).toBe(false);
});

/**
 * The bar's toggle cycles system, then light, then dark. From a light system
 * scheme the first press changes nothing anyone can see — light over light —
 * which is exactly why the second press is what these assert on.
 */
async function chooseDarkTheme(page: Page): Promise<void> {
  await page.getByRole("button", { name: /^Theme: / }).click();
  await page.getByRole("button", { name: /^Theme: / }).click();
}

test.describe("the theme override", () => {
  test.use({ colorScheme: "light" });

  test("wins over the system when the reader picks dark explicitly", async ({
    offlinePage: page,
    basemapRequests,
  }) => {
    await openLibrary(page);
    expect(await backgroundOfBody(page)).toBe(LIGHT_SURFACE);

    // From the map itself: the scheme is in the bar, so it is reached without
    // leaving the page being read.
    await chooseDarkTheme(page);

    // The page repaints in JavaScript-set state rather than a stylesheet
    // recomputing on its own, so this is read back rather than asserted the
    // instant the button is pressed.
    await expect.poll(() => backgroundOfBody(page)).toBe(DARK_SURFACE);
    // The same override reaches the basemap, which cannot follow CSS at all —
    // see the dark-scheme test above for why the request is the proof.
    await expect.poll(() => basemapRequests.some((url) => url.includes("dark"))).toBe(true);
  });

  test("holds across a page the control is not on", async ({ offlinePage: page }) => {
    await openLibrary(page);
    await chooseDarkTheme(page);
    await expect.poll(() => backgroundOfBody(page)).toBe(DARK_SURFACE);

    await page.getByRole("link", { name: "Settings" }).click();

    await expect.poll(() => backgroundOfBody(page)).toBe(DARK_SURFACE);
    // And the bar on that page agrees about which scheme is in force, rather
    // than each mounting of the toggle keeping a choice of its own.
    await expect(
      page.getByRole("button", { name: "Theme: dark. Switch to system." }),
    ).toBeVisible();
  });

  test("survives a reload", async ({ offlinePage: page, baseURL }) => {
    await openLibrary(page);
    await chooseDarkTheme(page);
    await expect.poll(() => backgroundOfBody(page)).toBe(DARK_SURFACE);

    await page.goto(baseURL ?? "");
    await expect.poll(() => backgroundOfBody(page)).toBe(DARK_SURFACE);
  });
});

test.describe("on a narrow viewport", () => {
  test.use({ viewport: { width: 375, height: 667 } });

  test("the panel gives up its fixed width and the map keeps the page", async ({
    offlinePage: page,
  }) => {
    await openLibrary(page);
    await (await openSearch(page)).fill("rhine");

    const panel = await page.getByRole("dialog", { name: "Route library" }).boundingBox();
    const map = await mapRegion(page).boundingBox();
    expect(panel).not.toBeNull();
    expect(map).not.toBeNull();
    if (!panel || !map) {
      return;
    }
    // Below the one breakpoint the panel is no longer the 436 px column it is on
    // a desktop, and neither it nor the map runs off the side.
    expect(panel.width).toBeLessThan(436);
    expect(panel.x + panel.width).toBeLessThanOrEqual(375);
    expect(map.width).toBeLessThanOrEqual(375);
  });

  /*
   * The scheme is still a control here rather than something only a wide
   * viewport gets. It sits at the same end of the bar as the session pill,
   * which at this width is already reached by scrolling the bar sideways — the
   * bar has never fitted 375 px, and this asserts the toggle is no worse off
   * than the mark beside it, not that either is on screen.
   */
  test("the colour scheme is still in the bar", async ({ offlinePage: page }) => {
    await openLibrary(page);

    const toggle = page.getByRole("button", { name: /^Theme: / });
    await expect(toggle).toHaveCount(1);
    await toggle.click();
    await expect(page.getByRole("button", { name: "Theme: light. Switch to dark." })).toHaveCount(
      1,
    );
  });

  // The tile credit is read out of a style document the page fetched, which is
  // why this is asked in a real browser rather than in jsdom.
  test("the settings page credits every data source", async ({ offlinePage: page }) => {
    await openLibrary(page);
    await page.getByRole("link", { name: "Settings" }).click();

    const credit = page.getByText(BASEMAP_ATTRIBUTION_TEXT);
    await expect(credit).toHaveText(BASEMAP_ATTRIBUTION_TEXT);
    // The provider wrapped that in a link. The page took the words and left the
    // markup, which is the rule the credit is read out of the document under.
    await expect(credit.locator("a")).toHaveCount(0);

    await expect(page.getByText(/Surface data © OpenStreetMap contributors/)).toBeVisible();
    await expect(page.getByText(/Weather data by Open-Meteo/)).toBeVisible();
  });

  test("the map itself carries no credit", async ({ offlinePage: page }) => {
    await openLibrary(page);

    await expect(page.getByText(BASEMAP_ATTRIBUTION_TEXT)).toHaveCount(0);
    await expect(page.getByRole("button", { name: /the map credit/ })).toHaveCount(0);
  });

  test("a route still shows its map and its chart", async ({ offlinePage: page }) => {
    await openRoute(page, LOOP_ROUTE.provider, LOOP_ROUTE.sourceRouteId, LOOP_ROUTE.stageOrder);

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

/*
 * Selection, which is a CSS rule and therefore has no answer in jsdom: what a
 * double click leaves behind is decided by the layout engine that worked out
 * what the word under the pointer was.
 */
test.describe("text selection", () => {
  test("a double click on the page's own text selects nothing", async ({ offlinePage: page }) => {
    await openLibrary(page);
    await page.getByRole("link", { name: "Settings" }).click();

    // A run of ordinary prose, well away from the map — which has had its own
    // selection turned off since long before the document did.
    await page.getByText(/Weather data by Open-Meteo/).dblclick();

    expect(await page.evaluate(() => window.getSelection()?.toString() ?? "")).toBe("");
  });

  test("a field still selects the text typed into it", async ({ offlinePage: page }) => {
    await openLibrary(page);
    const search = await openSearch(page);
    await search.fill("rhine");

    // Triple click rather than `selectText`, which selects through the DOM and
    // would pass whatever the stylesheet says.
    await search.click({ clickCount: 3 });

    const selected = await search.evaluate((field: HTMLInputElement) =>
      field.value.slice(field.selectionStart ?? 0, field.selectionEnd ?? 0),
    );
    expect(selected).toBe("rhine");
  });

  test.describe("in the drawer the panels dock into", () => {
    test.use({ viewport: { width: 375, height: 667 } });

    // The drawer is registry output and asks for `select-text` on its content.
    // index.css overrules it, and what wins is decided by the cascade rather
    // than by either file on its own.
    test("the drawer does not hand selection back", async ({ offlinePage: page }) => {
      await openLibrary(page);
      await openWorkspace(page);

      const content = page.locator('[data-slot="drawer-content"]');
      await expect(content).toBeVisible();
      expect(await content.evaluate((node) => getComputedStyle(node).userSelect)).toBe("none");
    });
  });
});
