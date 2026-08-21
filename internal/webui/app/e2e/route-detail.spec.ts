/**
 * One route: the map, the chart, and the two of them as one instrument.
 *
 * The interactions here are the reason this suite exists. Each of them spans a
 * real map, a real chart and the state they share, and each one ends in paint —
 * a dimmed route, a lit class, a redrawn window — which is the part no jsdom test
 * can see.
 */

import { expect, mapRegion, openRoute, profileScrubber, settleMap, test } from "./fixtures";

/** A straight three-band route: the simplest ground to point at. */
const LINE_ROUTE = { routeId: 4101, stageOrder: 3 };
/** The loop, whose classification covers all six surface classes. */
const LOOP_ROUTE = { routeId: 4102, stageOrder: 1 };
/** The short link, which was never classified and has no profile at all. */
const UNCLASSIFIED_ROUTE = { routeId: 4103, stageOrder: 1 };

test("the route draws its map, its facts and its profile", async ({ offlinePage: page }) => {
  await openRoute(page, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  await expect(page.locator(".maplibregl-canvas")).toBeVisible();
  await expect(page.locator(".route-panel__figures")).toContainText("km");
  // What the map says to a reader who cannot see it: the same start, finish and
  // direction the markers and chevrons draw.
  await expect(mapRegion(page)).toContainText("Starts and finishes");
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();
  // The scale control only has a distance to print once the camera has a zoom,
  // so this is the map reporting that it really rendered rather than that its
  // container exists.
  await expect(page.locator(".maplibregl-ctrl-scale")).toContainText(/\d/);
});

test("the chart answers the arrow keys and says where it is", async ({ offlinePage: page }) => {
  await openRoute(page, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  const scrubber = profileScrubber(page);
  await scrubber.focus();
  await expect(scrubber).toBeFocused();
  const start = Number(await scrubber.getAttribute("aria-valuenow"));

  await scrubber.press("ArrowRight");
  await scrubber.press("ArrowRight");
  await scrubber.press("ArrowRight");

  const moved = Number(await scrubber.getAttribute("aria-valuenow"));
  expect(moved).toBeGreaterThan(start);
  // The readout is a live region, so a reader who cannot see the marker is told
  // the same thing the marker shows.
  await expect(page.locator(".elevation-profile__readout")).toContainText("m");

  await scrubber.press("ArrowLeft");
  expect(Number(await scrubber.getAttribute("aria-valuenow"))).toBeLessThan(moved);
});

test("dragging across the chart zooms into that stretch, and Escape leaves it", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  const chart = page.locator(".elevation-profile");
  const box = await chart.boundingBox();
  expect(box).not.toBeNull();
  if (!box) {
    return;
  }
  const y = box.y + box.height / 2;
  await page.mouse.move(box.x + box.width * 0.3, y);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.6, y, { steps: 8 });
  await page.mouse.up();

  await expect(chart).toHaveAttribute("data-zoomed", "true");
  // The overview's summary keeps the stretch visible even when the chart holding
  // the way back is folded away, so it is the honest place to read the window
  // from.
  await expect(page.locator(".elevation-panel__summary")).toContainText("km shown");

  await page.keyboard.press("Escape");
  await expect(chart).not.toHaveAttribute("data-zoomed", "true");
  await expect(page.locator(".elevation-panel__summary")).not.toContainText("km shown");
});

test("dragging along the route picks the same stretch off the map", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  // A straight route's midpoint is the centre of the bounds the map fitted, so
  // the centre of the canvas is on the line — which is what tells a selection
  // from a pan. A drag that began off the line would pan the map instead, and
  // this assertion is what would catch that distinction being lost.
  const region = mapRegion(page);
  const box = await region.boundingBox();
  expect(box).not.toBeNull();
  if (!box) {
    return;
  }
  const centreX = box.x + box.width / 2;
  const centreY = box.y + box.height / 2;
  await page.mouse.move(centreX, centreY);
  await page.mouse.down();
  await page.mouse.move(centreX + 120, centreY + 40, { steps: 10 });
  await page.mouse.up();

  await expect(page.locator(".elevation-panel__summary")).toContainText("km shown");
  await expect(page.locator(".elevation-profile")).toHaveAttribute("data-zoomed", "true");

  // Escape over the map returns the whole route, the same way out the chart's own
  // control offers.
  await page.keyboard.press("Escape");
  await expect(page.locator(".elevation-panel__summary")).not.toContainText("km shown");
});

test("picking a surface class out of the key repaints the map", async ({ offlinePage: page }) => {
  await openRoute(page, LOOP_ROUTE.routeId, LOOP_ROUTE.stageOrder);
  const before = await settleMap(page);

  const key = page.getByRole("list", { name: "Surface classes" });
  await expect(key).toBeVisible();
  const gravel = key.getByRole("button", { name: /^Gravel/ });
  await gravel.click();

  await expect(gravel).toHaveAttribute("aria-pressed", "true");
  const after = await settleMap(page);
  // The map dims everything but the chosen class, so the canvas has to differ.
  // Comparing the map against itself within the run is what makes this a visual
  // assertion without a stored image to go stale.
  expect(after.equals(before)).toBe(false);

  await gravel.click();
  await expect(gravel).toHaveAttribute("aria-pressed", "false");
});

test("a route nobody classified says so rather than showing an empty key", async ({
  offlinePage: page,
}) => {
  await openRoute(page, UNCLASSIFIED_ROUTE.routeId, UNCLASSIFIED_ROUTE.stageOrder);

  await expect(page.getByText("Surface not classified yet.")).toBeVisible();
  // The same route has no elevation either, which is a second absence the page
  // has to state instead of drawing a flat line through it.
  await expect(page.locator(".elevation-profile__absent")).toContainText("no elevation data");
});

/*
 * The chart is a panel over the map rather than a column beside it, so putting
 * it away is how a reader gets the ground back — and the two figures it existed
 * to give at a glance have to survive that.
 */
test("the profile folds into a pill that still carries its figures", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  const panel = page.locator(".elevation-panel");
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();

  await page.getByRole("button", { name: "Hide the profile" }).click();

  await expect(panel).toHaveAttribute("data-collapsed", "true");
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeHidden();
  await expect(panel.locator(".elevation-panel__summary")).toContainText("m");
  // The route is still open behind it: the chart was put away, not the route.
  await expect(page.locator(".route-panel__figures")).toContainText("km");

  await page.getByRole("button", { name: "Show the profile" }).click();
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();
});

test("the way back to the library is reachable from the keyboard", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  const back = page.getByRole("button", { name: "← Back to search" });
  await back.focus();
  await expect(back).toBeFocused();
  await back.press("Enter");

  await expect(page).toHaveURL(/\/$/);
  // The library is a map, not a list: what proves the way back arrived is the
  // search over everything rather than a column of cards.
  await expect(page.getByRole("searchbox", { name: "Search the route library" })).toBeVisible();
});
