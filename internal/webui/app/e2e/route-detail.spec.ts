/**
 * One route: the map, the chart, and the two of them as one instrument.
 *
 * The interactions here are the reason this suite exists. Each of them spans a
 * real map, a real chart and the state they share, and each one ends in paint —
 * a dimmed route, a lit class, a redrawn window — which is the part no jsdom test
 * can see.
 */

import type { Page } from "@playwright/test";
import { expect, mapRegion, openRoute, profileScrubber, settleMap, test } from "./fixtures";

/** A straight three-band route: the simplest ground to point at. */
const LINE_ROUTE = { provider: "veloplanner", routeId: 4101, stageOrder: 3 };
/** A hill route, used where the climbs disclosure is the subject. */
const CLIMB_ROUTE = { provider: "veloplanner", routeId: 4101, stageOrder: 1 };
/** The loop, whose classification covers all six surface classes. */
const LOOP_ROUTE = { provider: "veloplanner", routeId: 4102, stageOrder: 1 };
/** The short link, which was never classified and has no profile at all. */
const UNCLASSIFIED_ROUTE = { provider: "veloplanner", routeId: 4103, stageOrder: 1 };

function routePanel(page: Page) {
  return page
    .getByRole("button", { name: /^Search \d+ routes?$/ })
    .locator("xpath=ancestor::section");
}

function profile(page: Page) {
  return profileScrubber(page).locator("..");
}

test("the route draws its map, its facts and its profile", async ({ offlinePage: page }) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  await expect(page.locator(".maplibregl-canvas")).toBeVisible();
  await expect(routePanel(page)).toContainText("km");
  // What the map says to a reader who cannot see it: the same start, finish and
  // direction the markers and chevrons draw.
  await expect(mapRegion(page)).toContainText("Starts and finishes");
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();
  // The scale control only has a distance to print once the camera has a zoom,
  // so this is the map reporting that it really rendered rather than that its
  // container exists.
  await expect(page.locator(".maplibregl-ctrl-scale")).toContainText(/\d/);
});

test("climbs start folded and expand on demand", async ({ offlinePage: page }) => {
  await openRoute(page, CLIMB_ROUTE.provider, CLIMB_ROUTE.routeId, CLIMB_ROUTE.stageOrder);

  const toggle = page.getByRole("button", { name: /^Show \d+ climbs?$/ });
  await expect(toggle).toBeVisible();
  await toggle.click();
  const expanded = page.getByRole("button", { name: /^Hide \d+ climbs?$/ });
  await expect(expanded).toBeVisible();
  await expect(
    expanded.locator("xpath=ancestor::section").getByRole("listitem").first(),
  ).toBeVisible();
});

test("the chart answers the arrow keys and says where it is", async ({ offlinePage: page }) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

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
  await expect(scrubber).toHaveAttribute("aria-valuetext", /metres/);

  await scrubber.press("ArrowLeft");
  expect(Number(await scrubber.getAttribute("aria-valuenow"))).toBeLessThan(moved);
});

test("dragging across the chart zooms into that stretch, and Escape leaves it", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  const chart = profile(page);
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
  // The header row keeps the stretch visible even when the chart holding the way
  // back is folded away, so it is the honest place to read the window from.
  await expect(page.getByLabel("Elevation summary")).toContainText("km shown");

  await page.keyboard.press("Escape");
  await expect(chart).not.toHaveAttribute("data-zoomed", "true");
  await expect(page.getByLabel("Elevation summary")).not.toContainText("km shown");
});

test("dragging along the route picks the same stretch off the map", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  /*
   * A straight route's midpoint is the centre of the bounds the map fitted, so
   * that point is on the line — which is what tells a selection from a pan. A
   * drag that began off the line would pan the map instead, and this assertion
   * is what would catch that distinction being lost.
   *
   * Not the centre of the canvas: the camera frames a route in the part of the
   * map no panel is standing on, so the midpoint sits at the centre of what the
   * column leaves.
   */
  const region = mapRegion(page);
  const box = await region.boundingBox();
  expect(box).not.toBeNull();
  if (!box) {
    return;
  }
  const column = await routePanel(page).boundingBox();
  const centreX = ((column ? column.x + column.width : box.x) + box.x + box.width) / 2;
  const centreY = box.y + box.height / 2;
  await page.mouse.move(centreX, centreY);
  await page.mouse.down();
  await page.mouse.move(centreX + 120, centreY + 40, { steps: 10 });
  await page.mouse.up();

  await expect(page.getByLabel("Elevation summary")).toContainText("km shown");
  await expect(profile(page)).toHaveAttribute("data-zoomed", "true");

  // Escape over the map returns the whole route, the same way out the chart's own
  // control offers.
  await page.keyboard.press("Escape");
  await expect(page.getByLabel("Elevation summary")).not.toContainText("km shown");
});

test("picking a surface class out of the key repaints the map", async ({ offlinePage: page }) => {
  await openRoute(page, LOOP_ROUTE.provider, LOOP_ROUTE.routeId, LOOP_ROUTE.stageOrder);
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
  await openRoute(
    page,
    UNCLASSIFIED_ROUTE.provider,
    UNCLASSIFIED_ROUTE.routeId,
    UNCLASSIFIED_ROUTE.stageOrder,
  );

  await expect(page.getByText("Surface not classified yet.")).toBeVisible();
  // The same route has no elevation either, which is a second absence the page
  // has to state instead of drawing a flat line through it.
  await expect(page.getByText(/no elevation data/)).toBeVisible();
});

/*
 * The chart is a row of the route's card, so putting it away is how a reader
 * gets the rest of the card and the map back — and the two figures it existed to
 * give at a glance have to survive that.
 */
test("the profile folds to a row that still carries its figures", async ({ offlinePage: page }) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  const section = page.getByRole("region", { name: "Elevation" });
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();

  await page.getByRole("button", { name: "Hide the profile" }).click();

  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeHidden();
  await expect(section.getByLabel("Elevation summary")).toContainText("m");
  // The route is still open around it: the chart was put away, not the route.
  await expect(routePanel(page)).toContainText("km");

  await page.getByRole("button", { name: "Show the profile" }).click();
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();
});

/*
 * The reason the tooltip exists: folding the profile away is exactly when a
 * reader wants the map to itself, and before this it left a hover with a dot
 * and no numbers at all.
 */
test("hovering the route labels the position while the profile is folded away", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  /*
   * Landed on the route while the profile is still open, at the same point
   * "dragging along the route picks the same stretch off the map" uses. The
   * card is folded from the keyboard rather than by clicking it, so the mouse
   * itself never leaves the map — a real click would move the pointer to the
   * button first and report the very `mouseout` this is checking survives.
   */
  const region = mapRegion(page);
  const box = await region.boundingBox();
  const panel = await routePanel(page).boundingBox();
  expect(box).not.toBeNull();
  if (!box) {
    return;
  }
  const x = ((panel ? panel.x + panel.width : box.x) + box.x + box.width) / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  // Proof the point really is on the line, before the readout it would have
  // shown this on is folded away.
  await expect(profileScrubber(page)).toHaveAttribute("aria-valuetext", /percent/);

  const collapse = page.getByRole("button", { name: "Hide the profile" });
  await collapse.focus();
  await collapse.press("Enter");
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeHidden();

  // How far along, how high, how steep — the distance still to ride is the bar
  // under them rather than a second figure beside them.
  const tooltip = page.locator(".route-position-tooltip");
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toContainText(/\d/);
  await expect(tooltip).toContainText("%");
  // The stage's one classified band, named from the same styles the key uses.
  await expect(tooltip).toContainText("Asphalt");

  // Moving onto the panel takes the pointer off the map, the same way leaving
  // the canvas through `mouseout` does.
  const foldedPanel = await routePanel(page).boundingBox();
  expect(foldedPanel).not.toBeNull();
  if (foldedPanel) {
    await page.mouse.move(
      foldedPanel.x + foldedPanel.width / 2,
      foldedPanel.y + foldedPanel.height / 2,
    );
  }
  await expect(tooltip).toBeHidden();
});

test("the way back to the library is reachable from the keyboard", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  const back = page.getByRole("button", { name: /^Search \d+ routes?$/ });
  await back.focus();
  await expect(back).toBeFocused();
  await back.press("Enter");

  await expect(page).toHaveURL(/\/$/);
  // The library is a map, not a list: what proves the way back arrived is the
  // search over everything rather than a column of cards.
  await expect(page.getByRole("button", { name: "Search the route library" })).toBeVisible();
});

/*
 * There is no default start time — see lib/startTime.ts — so a reader has to
 * choose one before the strip draws anything, and the value it chooses proves
 * the forecast is really landing under the elevation chart rather than beside
 * it by coincidence.
 */
test("choosing a start time draws a forecast strip under the profile", async ({
  offlinePage: page,
}) => {
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  await expect(page.getByRole("img", { name: /Forecast along the way/ })).toHaveCount(0);

  const soon = new Date(Date.now() + 2 * 60 * 60 * 1000);
  await page.getByLabel("Ride start").fill(soon.toISOString().slice(0, 16));

  const strip = page.getByRole("img", { name: /Forecast along the way/ });
  await expect(strip).toBeVisible();
  // Attribution the licence requires wherever the forecast appears — see
  // components/ForecastStrip.tsx.
  await expect(page.getByText("Weather data by Open-Meteo.com")).toBeVisible();

  /*
   * The alignment is the whole point of drawing the strip here rather than
   * anywhere else, and it is invisible to every assertion above: the strip
   * shipped once measuring nothing, drawing itself at the fallback width and
   * stretching that over the card, which reads as a strip until you notice the
   * rain sitting a kilometre from the climb it falls on. Both charts plot
   * against the same measured width, so their viewBoxes agree to the pixel or
   * one of them is measuring something the other is not.
   */
  const widths = {
    profile: (
      await page.getByRole("img", { name: /^Elevation profile of / }).getAttribute("viewBox")
    )?.split(" ")[2],
    strip: (await strip.getAttribute("viewBox"))?.split(" ")[2],
  };
  expect(widths.strip, "the strip and the profile plot at the same width").toBe(widths.profile);
});

/*
 * A stage with no predicted moving time — the same one that has no elevation
 * profile at all — has nothing a forecast sample could be timed against, and
 * the strip has to say nothing rather than guess.
 */
test("a stage with no predicted moving time shows no forecast strip", async ({
  offlinePage: page,
}) => {
  await openRoute(
    page,
    UNCLASSIFIED_ROUTE.provider,
    UNCLASSIFIED_ROUTE.routeId,
    UNCLASSIFIED_ROUTE.stageOrder,
  );

  const soon = new Date(Date.now() + 2 * 60 * 60 * 1000);
  await page.getByLabel("Ride start").fill(soon.toISOString().slice(0, 16));

  await expect(page.getByRole("img", { name: /Forecast along the way/ })).toHaveCount(0);
});
