/**
 * The entry page: the map of everything, the search over it, and the way into a
 * route.
 *
 * These are the paths a component test cannot reach — the library is a MapLibre
 * map with a line per route, picking one moves a real camera, and the panel and
 * the cartography are only worth asserting against each other over the whole
 * page they share.
 */

import type { Page } from "@playwright/test";
import { expect, mapRegion, openLibrary, openSearch, settleMap, test } from "./fixtures";

const DEMO_TITLES = [
  "Synthetic Rhine Traverse — Valley floor",
  "Synthetic Rhine Traverse — Forest ramps",
  "Synthetic Rhine Traverse — Run to the border",
  "Synthetic Kaiserstuhl Loop",
  "Synthetic Station Link",
  "Synthetic Summit Ascent",
  "Synthetic Foothill Circuit",
];

/** The demo's loop, which the map is asked to fly to. */
const LOOP = {
  provider: "veloplanner",
  routeId: 4102,
  stageOrder: 1,
  title: "Synthetic Kaiserstuhl Loop",
};

/** Result rows are buttons named by their decorative route shape and route title. */
function results(page: Page) {
  return page.getByRole("button", { name: /^Shape of Synthetic/ });
}

test("the entry page is the library, drawn", async ({ offlinePage: page }) => {
  await openLibrary(page);

  // The bar across the top and the map under it, with the search pill the only
  // panel standing on the map at rest.
  await expect(mapRegion(page)).toBeVisible();
  await expect(page.locator(".maplibregl-canvas")).toBeVisible();
  await expect(page.getByText("domestique")).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary" })).toBeVisible();
  await expect(page.getByRole("link", { name: /^Sync/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Search the route library" })).toBeVisible();
  await expect(await openSearch(page)).toHaveAttribute(
    "placeholder",
    `Search ${DEMO_TITLES.length} routes`,
  );
  // Nothing is listed until something is asked: the results column is what a
  // search or a selection grows.
  await expect(results(page)).toHaveCount(0);
  // The scale control only has a distance to print once the camera has a zoom,
  // so this is the map reporting that it framed the library rather than that its
  // container exists.
  await expect(page.locator(".maplibregl-ctrl-scale")).toContainText(/\d/);
});

test("the Tabler zoom controls move the map", async ({ offlinePage: page }) => {
  await openLibrary(page);
  const before = await settleMap(page);

  await page.getByRole("button", { name: "Zoom in" }).click();

  expect((await settleMap(page)).equals(before)).toBe(false);
});

test("keeps map actions, scale, and attribution in their own corners", async ({
  offlinePage: page,
}) => {
  await openLibrary(page);

  const map = await mapRegion(page).boundingBox();
  const locate = await page.getByRole("button", { name: "Find my location" }).boundingBox();
  const zoom = await page.getByRole("button", { name: "Zoom in" }).boundingBox();
  const credit = await page.locator(".map-credits").boundingBox();
  expect(map).not.toBeNull();
  expect(locate).not.toBeNull();
  expect(zoom).not.toBeNull();
  expect(credit).not.toBeNull();
  if (!map || !locate || !zoom || !credit) {
    throw new Error("expected the map controls to have been laid out");
  }

  for (const control of [locate, zoom]) {
    expect(control.x).toBeGreaterThan(map.x + map.width / 2);
    expect(control.y).toBeLessThan(map.y + map.height / 2);
  }
  expect(credit.x).toBeGreaterThan(map.x + map.width / 2);
  expect(credit.y).toBeGreaterThan(map.y + map.height / 2);
  await expect(page.locator(".maplibregl-ctrl-scale")).toHaveCSS(
    "background-color",
    "rgba(0, 0, 0, 0)",
  );
  await expect(page.locator(".maplibregl-ctrl-scale")).toHaveCSS("border-bottom-style", "solid");
});

// The mixed case: a library assembled from more than one source, and a reader
// telling its stages apart by more than the row they happen to sit in.
test("a search can narrow the library to one route", async ({ offlinePage: page }) => {
  await openLibrary(page);
  const search = await openSearch(page);

  await search.fill("komoot");

  await expect(results(page)).toHaveCount(1);
  await expect(results(page)).toContainText("Synthetic Foothill Circuit");

  await search.fill("kaiserstuhl");
  await expect(results(page)).toContainText("Synthetic Kaiserstuhl Loop");
});

test("searching grows a column of what is left", async ({ offlinePage: page }) => {
  await openLibrary(page);
  const search = await openSearch(page);

  await search.fill("rhine");

  await expect(results(page)).toHaveCount(3);

  await search.fill("forest");

  await expect(results(page)).toHaveText([/Forest ramps/]);

  await search.fill("nothing is called this");

  await expect(results(page)).toHaveCount(0);
  await expect(page.getByText("Nothing here is called that.")).toBeVisible();
});

test("nothing a reader types leaves the page", async ({ offlinePage: page }) => {
  await openLibrary(page);
  const asked: string[] = [];
  page.on("request", (request) => asked.push(request.url()));

  await (await openSearch(page)).fill("kaiserstuhl");
  await expect(results(page)).toHaveCount(1);

  // Narrowing happens in the browser over the listing the page already holds,
  // which is what keeps route names out of an access log.
  expect(asked.filter((url) => url.toLowerCase().includes("kaiserstuhl"))).toEqual([]);
});

test("picking a route lifts it out of the library and opens its card", async ({
  offlinePage: page,
}) => {
  await openLibrary(page);
  const before = await settleMap(page);

  await (await openSearch(page)).fill("kaiserstuhl");
  await page.getByRole("button", { name: new RegExp(LOOP.title) }).click();

  // The row is replaced by the card, so the column never says the same route
  // twice.
  await expect(page.getByRole("heading", { name: LOOP.title })).toBeVisible();
  await expect(page.getByRole("button", { name: new RegExp(LOOP.title) })).toHaveCount(0);
  await expect(page.getByTestId("gradient-mix").locator("span").first()).toBeVisible();

  // The camera followed the selection and the accent went on: comparing the map
  // against itself within the run is a visual assertion with no stored image to
  // go stale.
  const after = await settleMap(page);
  expect(after.equals(before)).toBe(false);
});

test("the card is the way into a route, and the route takes the same column", async ({
  offlinePage: page,
}) => {
  await openLibrary(page);
  await (await openSearch(page)).fill("kaiserstuhl");
  await page.getByRole("button", { name: new RegExp(LOOP.title) }).click();

  await page.getByRole("button", { name: "Open route" }).click();

  // The route is a panel over the same map, not a page of its own — but it is in
  // the address, so it is still a view that can be sent to someone else.
  await expect(page).toHaveURL(
    new RegExp(`/\\?route=${LOOP.provider}%2F${LOOP.routeId}%2F${LOOP.stageOrder}$`),
  );
  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "Search the route library" })).toBeHidden();
  await settleMap(page);
});

// The address a route page has now that a stage's identity names its provider.
// This is the link the service itself hands out, so it is the one that has to
// land before any of the older spellings below do.
test("a link to the route page lands on the route", async ({ offlinePage: page }) => {
  await page.goto(`/routes/${LOOP.provider}/${LOOP.routeId}/${LOOP.stageOrder}`);

  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
  await expect(page).toHaveURL(
    new RegExp(`/\\?route=${LOOP.provider}%2F${LOOP.routeId}%2F${LOOP.stageOrder}$`),
  );
});

// The address routes were linked by before they became a panel. Old links and
// bookmarks have to land on the route rather than on the library.
test("a link to the old route page lands on the route", async ({ offlinePage: page }) => {
  await page.goto(`/routes/${LOOP.routeId}/${LOOP.stageOrder}`);

  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
  await expect(page).toHaveURL(
    new RegExp(`/\\?route=${LOOP.provider}%2F${LOOP.routeId}%2F${LOOP.stageOrder}$`),
  );
});

// The query form that address redirects into, as a link of its own — the shape
// a bookmark actually holds, in the two-part spelling it held before providers.
test("a bookmarked two-part address lands on the route", async ({ offlinePage: page }) => {
  await page.goto(`/?route=${LOOP.routeId}%2F${LOOP.stageOrder}`);

  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
});

/**
 * Points at a route on the map, wherever one happens to be.
 *
 * The framing decides where any one route falls, so the pointer is swept across
 * the map until the cursor says it is over a line — which is the same promise
 * the reader is given before they click. It leaves the pointer where it found
 * one, ready to click.
 */
async function pointAtALine(page: Page): Promise<{ x: number; y: number }> {
  const box = await mapRegion(page).boundingBox();
  expect(box).not.toBeNull();
  if (!box) {
    throw new Error("expected the map to have been laid out");
  }
  const canvas = page.locator(".maplibregl-canvas");
  for (const yFraction of [0.25, 0.4, 0.55, 0.7]) {
    const y = box.y + box.height * yFraction;
    for (let step = 1; step < 40; step += 1) {
      const x = box.x + (box.width * step) / 40;
      await page.mouse.move(x, y);
      await page.waitForTimeout(60);
      if ((await canvas.evaluate((node) => node.style.cursor)) === "pointer") {
        return { x, y };
      }
    }
  }
  throw new Error("expected a route somewhere on the map");
}

/*
 * The map is the library, so a line on it is the route itself — and picking one
 * off the ground takes the same two steps the column does. The first click says
 * which route was hit, and the second is the map's own way of saying yes.
 *
 * The two clicks cannot be at the same point: the first one flies the camera to
 * what it picked, so the line has to be found again afterwards. What is found
 * is not guaranteed to be the same line — routes in this library overlap, and a
 * scan returns whichever one is under the first point it tries — and landing on
 * a neighbour is a first click on that neighbour rather than a second on this
 * route. That is the map behaving correctly, so the loop below follows the
 * selection wherever it goes and asks for the second click against whatever the
 * panel names at the time, instead of assuming the scan came back to the same
 * route.
 */
/*
 * Longer than the default, because this test's own bounds are longer than it.
 * `pointAtALine` hunts for a line by walking a grid of 4 rows by 39 steps,
 * pausing 60ms at each to ask the canvas what is under the pointer — up to 9.4s
 * to find one. The search below runs it once, then up to five more times with a
 * `settleMap` (up to 5s) before each, which puts the bounded worst case around
 * 81s. It usually lands in a fraction of that, which is why 60s held for so
 * long, but a run that has to hunt was always going to exceed it. A slower
 * machine only decides how often that happens.
 */
test("pointing at a line on the map picks that route out, twice to open it", async ({
  offlinePage: page,
}) => {
  test.setTimeout(120_000);
  await openLibrary(page);

  /** What the card in the column is currently about. */
  const picked = () => page.getByRole("heading", { level: 2 }).innerText();

  const first = await pointAtALine(page);
  await page.mouse.click(first.x, first.y);
  let title = await picked();

  // Bounded: each pass either opens the route or moves the selection to the one
  // it actually hit, and a library of seven routes cannot hand out new
  // neighbours for ever.
  for (let attempt = 0; attempt < 5; attempt += 1) {
    await settleMap(page);
    const next = await pointAtALine(page);
    await page.mouse.click(next.x, next.y);

    const opened = page.getByRole("region", { name: title });
    if (await opened.isVisible()) {
      await expect(opened).toBeVisible();
      await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();

      return;
    }

    // Not opened, so that click picked a different line out. It is now the
    // selected route, and the next pass is its second click.
    title = await picked();
  }

  throw new Error("expected two clicks on one line to open it");
});

test("the bar names the session the gate admitted", async ({ offlinePage: page }) => {
  await openLibrary(page);

  // The demo configures one authorised address and mints its own assertion for
  // it, so what the gate admitted here is what a deployment's gate admits: the
  // one address in the configuration.
  const pill = page.getByRole("button", { name: "Signed in as rider@example.test" });
  await expect(pill).toBeVisible();
  await expect(pill).toHaveText("R");

  await pill.click();

  const session = page.getByRole("dialog", { name: "Session" });
  await expect(session).toContainText("rider@example.test");
  // Nothing stands in front of the demo, so there is nowhere to sign out to and
  // the menu offers no way out rather than a link that would answer 404.
  await expect(page.getByRole("link", { name: /Sign out/ })).toHaveCount(0);
});

test("the menu bar says what sync is doing and is the way to it", async ({ offlinePage: page }) => {
  await openLibrary(page);

  await expect(page.getByText("domestique")).toBeVisible();
  const sync = page.getByRole("link", { name: /^Sync/ });
  await expect(sync).toHaveAttribute("href", "/sync");
  // The word is the destination and the dot beside it is the state. The demo has
  // one connected slot and one that never onboarded, so the dot is painted and
  // the link's name says why.
  await expect(sync).toHaveAttribute("data-tone", "alert");
  await expect(sync).toHaveAccessibleName("Sync \u00b7 A target is not connected");

  await sync.click();

  await expect(page).toHaveURL(/\/sync$/);
  await expect(page.getByRole("heading", { level: 1, name: "Sync" })).toBeVisible();
});
