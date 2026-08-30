/**
 * The bundle the service serves, reading the views the service returns.
 *
 * Everything here crosses the real HTTP boundary: the document and its assets
 * come from the Go embed handler, and every figure on the page was read by
 * generated operations from a response a real handler wrote from a real SQLite
 * row. A Go view and its generated client drifting apart surfaces as the page
 * failing to show what it fetched.
 */

import { mapRegion, openLibrary, openRoute, openSearch, openSync, settleMap } from "../fixtures";
import { callsTo, expect, test } from "./fixtures";

const LOOP_ROUTE = { provider: "veloplanner", sourceRouteId: 4102, stageOrder: 1 };
/** A route the seeded library does not contain. */
const ABSENT_ROUTE = { provider: "veloplanner", sourceRouteId: 9999, stageOrder: 1 };

test("the service serves a bundle the browser can boot", async ({ bundlePage: page }) => {
  const document = await page.goto("/");

  expect(document?.status()).toBe(200);
  const headers = document?.headers() ?? {};
  // The dev server applies none of this. A bundle that only works without a
  // content security policy works everywhere except in production.
  expect(headers["content-security-policy"], "the document carries the policy").toBeTruthy();
  expect(headers["cache-control"]).toContain("no-cache");
  expect(headers["x-content-type-options"]).toBe("nosniff");

  // The application mounted, which means the hashed module the document names was
  // served, parsed and run by the embed handler's file server.
  await expect(page.getByRole("button", { name: "Search the route library" })).toBeVisible();
  const asset = await page.locator("script[type='module']").first().getAttribute("src");
  expect(asset).toMatch(/^\/assets\/.+\.js$/);

  // A contract cutover replaces field names in place, so deployment verification
  // must include the browser request that discards its prior bundle.
  await page.reload({ waitUntil: "networkidle" });
  await expect(page.getByRole("button", { name: "Search the route library" })).toBeVisible();
});

test("the library is drawn from the routes view", async ({ bundlePage: page, apiCalls }) => {
  await openLibrary(page);

  // The listing is counted where the page states its size rather than in a
  // column of cards: the entry page draws the library on the map, and nothing is
  // listed until something is asked.
  await expect(await openSearch(page)).toHaveAttribute("placeholder", "Search 7 routes");
  await (await openSearch(page)).fill("kaiserstuhl");
  await page.getByRole("button", { name: /Synthetic Kaiserstuhl Loop/ }).click();
  // Distances come from `distanceMetres`, so a card with a figure on it proves
  // the generated route model matched the real response.
  await expect(
    page.getByRole("heading", { name: "Synthetic Kaiserstuhl Loop" }).locator(".."),
  ).toContainText("km");
  expect(callsTo(apiCalls, "GET", "/v1/routes").map((call) => call.status)).toContain(200);
  expect(callsTo(apiCalls, "GET", "/v1/webui/config").map((call) => call.status)).toContain(200);
});

test("a route's geometry and its surface reach the map", async ({ bundlePage: page, apiCalls }) => {
  await openRoute(page, LOOP_ROUTE.provider, LOOP_ROUTE.sourceRouteId, LOOP_ROUTE.stageOrder);

  await expect(page.getByRole("region", { name: "Synthetic Kaiserstuhl Loop" })).toBeVisible();
  await expect(page.locator(".maplibregl-canvas")).toBeVisible();
  await expect(page.getByRole("region", { name: "Synthetic Kaiserstuhl Loop" })).toContainText(
    "km",
  );
  // The coordinates, the bounding box and the surface ranges all came out of the
  // geometry view; a map that fitted its camera and a key that lists classes are
  // the two halves of that having been read.
  await expect(mapRegion(page)).toContainText("Starts and finishes");
  // The key sits behind the card's fold on this layer; where a later layer has
  // removed the fold, there is nothing to open and the key is already there.
  const foldedMixes = page.getByRole("button", { name: "Show gradient and surface" });
  if ((await foldedMixes.count()) > 0) {
    await foldedMixes.click();
  }
  await expect(page.getByRole("list", { name: "Surface classes" })).toBeVisible();

  const geometry = callsTo(
    apiCalls,
    "GET",
    `/v1/providers/${LOOP_ROUTE.provider}/sourceRoutes/${LOOP_ROUTE.sourceRouteId}/routes/${LOOP_ROUTE.stageOrder}/geometry`,
  );
  expect(geometry.map((call) => call.status)).toContain(200);
  await settleMap(page);
});

test("the status view drives the sync page", async ({ bundlePage: page, apiCalls }) => {
  await openSync(page);

  // Both configured slots, the schedule state of both halves, and a run summary:
  // the whole status view, rendered.
  await expect(page.getByText("rider-b", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Run now: Read from VeloPlanner" })).toBeVisible();
  await expect(page.getByLabel("Hourly: Read from VeloPlanner")).toBeVisible();
  await expect(page.getByRole("region", { name: "What the targets hold" })).toBeVisible();
  expect(callsTo(apiCalls, "GET", "/v1/status").map((call) => call.status)).toContain(200);
  // The history card is the third view the page reads, and it crosses the same
  // boundary as the other two.
  await expect(page.getByRole("region", { name: "What has happened" })).toBeVisible();
  expect(callsTo(apiCalls, "GET", "/v1/sync/runs").map((call) => call.status)).toContain(200);
});

test("a route the library does not hold is reported safely", async ({
  bundlePage: page,
  apiCalls,
  identity,
}) => {
  await page.goto(
    `/?route=${ABSENT_ROUTE.provider}%2F${ABSENT_ROUTE.sourceRouteId}%2F${ABSENT_ROUTE.stageOrder}`,
  );

  // The library is one listing, so an address naming a route that is not in it is
  // answered from what the page already has — with a sentence saying so, rather
  // than a code, a stack, or a blank map.
  await expect(page.getByText("No route at that address.")).toBeVisible();
  expect(
    callsTo(
      apiCalls,
      "GET",
      `/v1/providers/${ABSENT_ROUTE.provider}/sourceRoutes/${ABSENT_ROUTE.sourceRouteId}/routes/${ABSENT_ROUTE.stageOrder}/geometry`,
    ),
    "no request went out for a route the listing does not hold",
  ).toEqual([]);

  // The same request again, from outside the page, to state the shape the UI just
  // handled: an error envelope naming a code, and nothing about what went missing
  // inside the service.
  const envelope = await page.request.get(
    `/v1/providers/${ABSENT_ROUTE.provider}/sourceRoutes/${ABSENT_ROUTE.sourceRouteId}/routes/${ABSENT_ROUTE.stageOrder}/geometry`,
    { headers: identity },
  );
  expect(envelope.status()).toBe(404);
  expect(await envelope.json()).toEqual({
    error: { code: "not_found", message: expect.any(String) },
  });
});
