/**
 * What the page owes a reader who is not looking at it the way the design
 * assumed: an automated audit of the whole rendered page, a high-contrast
 * palette forced over it, and a phone-sized portrait viewport.
 *
 * None of the three can be asked in jsdom. axe judges most of its rules from
 * computed style, the Vitest suite loads no stylesheet, and neither
 * `forced-colors` nor a layout that has to wrap means anything without a layout
 * engine. The component-level scan in `src/components/accessibility.test.tsx` is
 * the floor under a single control; this is the same question asked of the page
 * those controls end up in.
 */

import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { expect, openLibrary, openRoute, openSync, test } from "./fixtures";

const LOOP_ROUTE = { routeId: 4102, stageOrder: 1 };

/**
 * Every violation axe can find in the page, as lines a failure can be read from.
 *
 * The map's own canvas is excluded: it is a WebGL surface MapLibre owns, and
 * what it draws is described by the labelled region around it rather than by
 * anything axe can inspect inside it.
 */
async function violations(page: Page): Promise<string[]> {
  const { violations: found } = await new AxeBuilder({ page })
    .exclude(".maplibregl-canvas-container")
    .analyze();

  return found.map(
    (violation) =>
      `${violation.id} (${violation.impact}): ${violation.help} — at ${violation.nodes
        .map((node) => node.target.join(" "))
        .join(", ")}`,
  );
}

/** Whether the page is wider than the window, which on a phone is a bug. */
function overflowsSideways(page: Page): Promise<boolean> {
  return page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
}

test("the entry page has nothing for axe to report", async ({ offlinePage: page }) => {
  await openLibrary(page);

  expect(await violations(page)).toEqual([]);
});

/*
 * The panel grown over the map is where most of the page's text lives, and it
 * is drawn over cartography rather than over a background this suite chose.
 */
test("the search panel has nothing for axe to report, open", async ({ offlinePage: page }) => {
  await openLibrary(page);
  await page.getByRole("searchbox", { name: "Search the route library" }).fill("kaiserstuhl");
  await page.getByRole("button", { name: /Synthetic Kaiserstuhl Loop/ }).click();

  expect(await violations(page)).toEqual([]);
});

test("the sync page has nothing for axe to report", async ({ offlinePage: page }) => {
  await openSync(page);

  expect(await violations(page)).toEqual([]);
});

test("a route has nothing for axe to report", async ({ offlinePage: page }) => {
  await openRoute(page, LOOP_ROUTE.routeId, LOOP_ROUTE.stageOrder);

  expect(await violations(page)).toEqual([]);
});

test.describe("in a forced-colours palette", () => {
  test.use({ contextOptions: { forcedColors: "active", reducedMotion: "reduce" } });

  test("the page is still readable and still passes its audit", async ({ offlinePage: page }) => {
    await openRoute(page, LOOP_ROUTE.routeId, LOOP_ROUTE.stageOrder);

    expect(await violations(page)).toEqual([]);
  });

  test("the marks whose colour is the information keep it", async ({ offlinePage: page }) => {
    await openRoute(page, LOOP_ROUTE.routeId, LOOP_ROUTE.stageOrder);

    // A forced palette repaints everything in two colours, which for the
    // gradient ramp would replace the encoding with a flat block: the colour of
    // a column *is* which band it is. These are the only marks exempted, and the
    // exemption is worth nothing unless they really do still differ.
    const fills = await page
      .locator(".elevation-profile__column")
      .evaluateAll((columns) => columns.map((column) => getComputedStyle(column).fill));
    expect(fills.length).toBeGreaterThan(1);
    expect(new Set(fills).size).toBeGreaterThan(1);

    // The chart's own ground has to stay behind them rather than being painted
    // over them, and the key's swatches have to keep the colours they explain.
    const swatches = await page
      .locator('[aria-label="Gradient bands"] [aria-hidden="true"]')
      .evaluateAll((elements) => elements.map((element) => getComputedStyle(element).background));
    expect(new Set(swatches).size).toBeGreaterThan(1);
  });
});

test.describe("on a phone-sized portrait viewport", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test("the library fits its width, at rest and with the panel grown", async ({
    offlinePage: page,
  }) => {
    await openLibrary(page);

    expect(await overflowsSideways(page)).toBe(false);

    // The panel is the shape most likely to push a phone sideways: it is a
    // fixed width on the desktop, and a card of figures inside it.
    await page.getByRole("searchbox", { name: "Search the route library" }).fill("kaiserstuhl");
    await page.getByRole("button", { name: /Synthetic Kaiserstuhl Loop/ }).click();

    expect(await overflowsSideways(page)).toBe(false);
  });

  test("the sync page fits its width, three cards deep", async ({ offlinePage: page }) => {
    await openSync(page);

    expect(await overflowsSideways(page)).toBe(false);
  });

  test("a route fits its width, key and profile included", async ({ offlinePage: page }) => {
    await openRoute(page, LOOP_ROUTE.routeId, LOOP_ROUTE.stageOrder);

    // The profile is a panel across the foot of the map, and the control that
    // puts it away is inside it: a header that ran off the side of a phone would
    // take the way back with it.
    await expect(page.getByRole("button", { name: "Hide the profile" })).toBeInViewport();
    expect(await overflowsSideways(page)).toBe(false);
  });

  test("every key chip is big enough to hit with a finger", async ({ offlinePage: page }) => {
    await openRoute(page, LOOP_ROUTE.routeId, LOOP_ROUTE.stageOrder);

    const chips = page.locator('[aria-label="Route key"] button');
    const count = await chips.count();
    expect(count).toBeGreaterThan(1);
    for (let index = 0; index < count; index += 1) {
      const box = await chips.nth(index).boundingBox();
      expect(box).not.toBeNull();
      // WCAG 2.2 SC 2.5.8: 24 by 24 is the floor, and the chips were under it on
      // the short axis.
      expect(box?.width ?? 0).toBeGreaterThanOrEqual(24);
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(24);
    }
  });
});
