/**
 * The entry page: the map of everything, the search over it, and the way into a
 * route.
 *
 * These are the paths a component test cannot reach — the library is a MapLibre
 * map with a line per route, picking one moves a real camera, and the panel and
 * the cartography are only worth asserting against each other over the whole
 * page they share.
 */

import { expect, mapRegion, openLibrary, settleMap, test } from "./fixtures";

const DEMO_TITLES = [
  "Synthetic Rhine Traverse — Valley floor",
  "Synthetic Rhine Traverse — Forest ramps",
  "Synthetic Rhine Traverse — Run to the border",
  "Synthetic Kaiserstuhl Loop",
  "Synthetic Station Link",
  "Synthetic Summit Ascent",
];

/** The demo's loop, which the map is asked to fly to. */
const LOOP = { routeId: 4102, stageOrder: 1, title: "Synthetic Kaiserstuhl Loop" };

test("the entry page is the library, drawn", async ({ offlinePage: page }) => {
  await openLibrary(page);

  // The map is the page: no header over it, and the only panel at rest is the
  // wordmark and the search pill.
  await expect(mapRegion(page)).toBeVisible();
  await expect(page.locator(".maplibregl-canvas")).toBeVisible();
  await expect(page.getByRole("link", { name: /domestique/ })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "Search the route library" })).toHaveAttribute(
    "placeholder",
    `Search ${DEMO_TITLES.length} routes`,
  );
  // Nothing is listed until something is asked: the results column is what a
  // search or a selection grows.
  await expect(page.locator(".result")).toHaveCount(0);
  // The scale control only has a distance to print once the camera has a zoom,
  // so this is the map reporting that it framed the library rather than that its
  // container exists.
  await expect(page.locator(".maplibregl-ctrl-scale")).toContainText(/\d/);
});

test("searching grows a column of what is left", async ({ offlinePage: page }) => {
  await openLibrary(page);
  const search = page.getByRole("searchbox", { name: "Search the route library" });

  await search.fill("rhine");

  await expect(page.locator(".result")).toHaveCount(3);
  await expect(page.getByText("3 of 6")).toBeVisible();

  await search.fill("forest");

  await expect(page.locator(".result__name")).toHaveText([
    "Synthetic Rhine Traverse — Forest ramps",
  ]);

  await search.fill("nothing is called this");

  await expect(page.locator(".result")).toHaveCount(0);
  await expect(page.getByText("Nothing here is called that.")).toBeVisible();
});

test("nothing a reader types leaves the page", async ({ offlinePage: page }) => {
  await openLibrary(page);
  const asked: string[] = [];
  page.on("request", (request) => asked.push(request.url()));

  await page.getByRole("searchbox", { name: "Search the route library" }).fill("kaiserstuhl");
  await expect(page.locator(".result")).toHaveCount(1);

  // Narrowing happens in the browser over the listing the page already holds,
  // which is what keeps route names out of an access log.
  expect(asked.filter((url) => url.toLowerCase().includes("kaiserstuhl"))).toEqual([]);
});

test("picking a route lifts it out of the library and opens its card", async ({
  offlinePage: page,
}) => {
  await openLibrary(page);
  const before = await settleMap(page);

  await page.getByRole("searchbox", { name: "Search the route library" }).fill("kaiserstuhl");
  await page.getByRole("button", { name: new RegExp(LOOP.title) }).click();

  // The row is replaced by the card, so the column never says the same route
  // twice.
  await expect(page.getByRole("heading", { name: LOOP.title })).toBeVisible();
  await expect(page.getByRole("button", { name: new RegExp(LOOP.title) })).toHaveCount(0);
  await expect(page.locator(".route-card__mix > span").first()).toBeVisible();

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
  await page.getByRole("searchbox", { name: "Search the route library" }).fill("kaiserstuhl");
  await page.getByRole("button", { name: new RegExp(LOOP.title) }).click();

  await page.getByRole("button", { name: "Open route" }).click();

  // The route is a panel over the same map, not a page of its own — but it is in
  // the address, so it is still a view that can be sent to someone else.
  await expect(page).toHaveURL(new RegExp(`/\\?route=${LOOP.routeId}%2F${LOOP.stageOrder}$`));
  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "Search the route library" })).toBeHidden();
  await settleMap(page);
});

// The address routes were linked by before they became a panel. Old links and
// bookmarks have to land on the route rather than on the library.
test("a link to the old route page lands on the route", async ({ offlinePage: page }) => {
  await page.goto(`/routes/${LOOP.routeId}/${LOOP.stageOrder}`);

  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/\\?route=${LOOP.routeId}%2F${LOOP.stageOrder}$`));
});

test("the wordmark says what sync is doing and is the way to it", async ({ offlinePage: page }) => {
  await openLibrary(page);

  const wordmark = page.getByRole("link", { name: /domestique/ });
  await expect(wordmark).toHaveAttribute("href", "/sync");
  // One line, never two: whether it wants the operator, and when it last
  // finished. The demo has one connected slot and one that never onboarded.
  await expect(page.locator(".wordmark__state")).toBeVisible();

  await wordmark.click();

  await expect(page).toHaveURL(/\/sync$/);
  await expect(page.getByRole("heading", { level: 1, name: "Sync" })).toBeVisible();
});
