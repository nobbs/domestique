/**
 * A route's own page, the search palette over the whole library, and the
 * addresses that lead into a route.
 *
 * The map and the palette's search are the paths a component test cannot
 * reach — a real MapLibre map, and a search whose narrowing is only worth
 * asserting against the network it did or did not reach.
 */

import { expect, followAccount, mapRegion, openRoute, paletteSearch, test } from "./fixtures";

/** The demo's loop, which a route page and a redirect both have to land on. */
const LOOP = {
  provider: "veloplanner",
  sourceRouteId: 4102,
  stageOrder: 1,
  title: "Synthetic Kaiserstuhl Loop",
};

test("a route's page draws its map, its panel and its dock", async ({ offlinePage: page }) => {
  await openRoute(page, LOOP.provider, LOOP.sourceRouteId, LOOP.stageOrder);

  await expect(mapRegion(page)).toBeVisible();
  await expect(page.locator(".maplibregl-canvas")).toBeVisible();
  await expect(page.getByText("domestique")).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary" })).toBeVisible();
  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
  await expect(page.getByRole("img", { name: /^Elevation profile of / })).toBeVisible();
  // The scale control only has a distance to print once the camera has a zoom,
  // so this is the map reporting that it really framed the route.
  await expect(page.locator(".maplibregl-ctrl-scale")).toContainText(/\d/);
});

// The mixed case: a library assembled from more than one source, and a reader
// telling its stages apart by more than the row they happen to sit in.
test("a search can narrow the palette to one route", async ({ offlinePage: page }) => {
  await page.goto("/activities");
  const search = await paletteSearch(page);

  await search.fill("komoot");

  await expect(page.getByRole("option")).toHaveCount(1);
  await expect(page.getByRole("option")).toContainText("Synthetic Foothill Circuit");

  await search.fill("kaiserstuhl");
  await expect(page.getByRole("option")).toContainText(LOOP.title);
});

test("nothing a reader types leaves the page", async ({ offlinePage: page }) => {
  await page.goto("/activities");
  const search = await paletteSearch(page);
  const asked: string[] = [];
  page.on("request", (request) => asked.push(request.url()));

  await search.fill("kaiserstuhl");
  await expect(page.getByRole("option")).toHaveCount(1);

  // Narrowing happens in the browser over the listing the page already holds,
  // which is what keeps route names out of an access log.
  expect(asked.filter((url) => url.toLowerCase().includes("kaiserstuhl"))).toEqual([]);
});

test("the bar names the session the gate admitted", async ({ offlinePage: page }) => {
  await page.goto("/activities");

  // The demo mints its own session, so what the gate admitted here is what a
  // deployment's gate admits: the account the session names.
  const pill = page.getByRole("button", { name: "Signed in as rider@example.test" });
  await expect(pill).toBeVisible();
  await expect(pill).toHaveText("R");

  await pill.click();

  const session = page.getByRole("menu");
  await expect(session).toContainText("rider@example.test");
});

test("the session says what sync is doing and leads to the account", async ({
  offlinePage: page,
}) => {
  await page.goto("/activities");

  // The demo has one connected slot and one that never onboarded, so the dot
  // on the session is painted and the menu item's name says why.
  const session = page.getByRole("button", { name: /^Signed in as/ });
  await expect(session).toHaveAttribute("data-tone", "alert");

  await followAccount(page);

  await expect(page).toHaveURL(/\/account\/sync$/);
  await expect(page.getByRole("heading", { level: 1, name: "Account" })).toBeVisible();
});

// The address the service itself hands out for a route.
test("a link to the route page lands on the route", async ({ offlinePage: page }) => {
  await page.goto(`/routes/${LOOP.provider}/${LOOP.sourceRouteId}/${LOOP.stageOrder}`);

  await expect(page).toHaveURL(
    new RegExp(`/routes/${LOOP.provider}/${LOOP.sourceRouteId}/${LOOP.stageOrder}$`),
  );
  await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
});

// The addresses a route was reached by before it had a page of its own — a
// two-part path and the `/?route=` query form redirect the same place.
test("the addresses a route was linked by before it had a page of its own still land there", async ({
  offlinePage: page,
}) => {
  for (const legacy of [
    `/routes/${LOOP.sourceRouteId}/${LOOP.stageOrder}`,
    `/?route=${LOOP.sourceRouteId}%2F${LOOP.stageOrder}`,
  ]) {
    await page.goto(legacy);

    await expect(page).toHaveURL(
      new RegExp(`/routes/${LOOP.provider}/${LOOP.sourceRouteId}/${LOOP.stageOrder}$`),
    );
    await expect(page.getByRole("region", { name: LOOP.title })).toBeVisible();
  }
});
