/**
 * The library page: the grid, its previews, and the way into a route.
 *
 * These are the paths a component test cannot reach — a card's preview is a
 * MapLibre map, and following a card is a client-side navigation into a view that
 * fetches its own geometry.
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

test("the library lists every stage in the demo", async ({ offlinePage: page }) => {
  await openLibrary(page);

  const cards = page.locator(".route-card");
  await expect(cards).toHaveCount(DEMO_TITLES.length);
  for (const title of DEMO_TITLES) {
    await expect(page.locator(".route-card__title", { hasText: title })).toBeVisible();
  }
  // The listing carries no geometry, so every card fetches its own: a card that
  // rendered its facts is one whose figures came from the stage summary.
  await expect(page.locator(".route-card__meta").first()).toContainText("km");
});

test("searching narrows the grid without leaving the page", async ({ offlinePage: page }) => {
  await openLibrary(page);

  await page.getByRole("searchbox", { name: "Search" }).fill("forest");

  await expect(page.locator(".route-card")).toHaveCount(1);
  await expect(page.locator(".route-card__title")).toHaveText(
    "Synthetic Rhine Traverse — Forest ramps",
  );
  // The stage keeps its place in the route it came from, even alone in the grid.
  await expect(page.locator(".route-card__stage")).toHaveText("Stage 2 of 3");
  await expect(page.getByText("Showing 1 of 6 stages")).toBeVisible();

  await page.getByRole("searchbox", { name: "Search" }).fill("nothing matches this");

  await expect(page.locator(".route-card")).toHaveCount(0);
  await expect(page.getByRole("status")).toContainText("No stages match");
  await page.getByRole("button", { name: "Clear search" }).click();
  await expect(page.locator(".route-card")).toHaveCount(DEMO_TITLES.length);
});

test("the grid can be reordered by distance", async ({ offlinePage: page }) => {
  await openLibrary(page);

  await page.getByRole("combobox", { name: "Sort by" }).selectOption("distance");

  // Longest first, which is what the order is called.
  const distances = await page.locator(".route-card__meta > span:first-child").allInnerTexts();
  const kilometres = distances.map((text) => Number.parseFloat(text));
  expect(kilometres).toEqual([...kilometres].sort((left, right) => right - left));
});

test("a card's preview becomes a map once it is in view", async ({ offlinePage: page }) => {
  await openLibrary(page);

  // The mini maps mount when a card nears the viewport, so the first row is
  // enough: the point is that a preview upgrades from the traced shape to a real
  // map, not that a long library mounts a WebGL context per card.
  await expect(page.locator(".maplibregl-canvas").first()).toBeVisible();
  const previews = await page.locator(".route-card .maplibregl-canvas").count();
  expect(previews).toBeGreaterThan(0);
});

test("following a card opens that stage", async ({ offlinePage: page }) => {
  await openLibrary(page);

  await page.locator(".route-card", { hasText: "Synthetic Kaiserstuhl Loop" }).click();

  await expect(page).toHaveURL(/\/routes\/4102\/1$/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Synthetic Kaiserstuhl Loop");
  await settleMap(page);
  await expect(mapRegion(page)).toBeVisible();
});

test("the page reports what the demo's two slots are doing", async ({ offlinePage: page }) => {
  await openLibrary(page);

  // One slot holds the whole library and one has never completed onboarding,
  // which is the pair `mise run demo` seeds and the state the onboarding path is
  // demonstrated from.
  await expect(page.getByRole("region", { name: /synchronisation/i })).toBeVisible();
  await expect(page.getByRole("button", { name: /Run now/ }).first()).toBeVisible();
  await expect(page.getByText("rider-b")).toBeVisible();
});
