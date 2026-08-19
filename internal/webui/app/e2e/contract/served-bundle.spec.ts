/**
 * The bundle the service serves, reading the views the service returns.
 *
 * Everything here crosses the real HTTP boundary: the document and its assets
 * come from the Go embed handler, and every figure on the page was parsed by the
 * shipped client out of a response a real handler wrote from a real SQLite row.
 * A Go view and its TypeScript parser drifting apart is the failure these tests
 * exist to catch, and it surfaces as the page failing to show what it fetched.
 */

import { mapRegion, openLibrary, openStage, settleMap } from "../fixtures";
import { callsTo, expect, test } from "./fixtures";

const LOOP_STAGE = { routeId: 4102, stageOrder: 1 };
/** A stage the seeded library does not contain. */
const ABSENT_STAGE = { routeId: 9999, stageOrder: 1 };

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
  await expect(page.locator(".route-card").first()).toBeVisible();
  const asset = await page.locator("script[type='module']").first().getAttribute("src");
  expect(asset).toMatch(/^\/assets\/.+\.js$/);
});

test("the library is drawn from the routes view", async ({ bundlePage: page, apiCalls }) => {
  await openLibrary(page);

  await expect(page.locator(".route-card")).toHaveCount(6);
  // Distances come from `distance_metres`, so a card with a figure on it is a
  // stage view this client could read. A renamed field would leave the parser
  // throwing and the page showing its error state instead.
  await expect(page.locator(".route-card__meta").first()).toContainText("km");
  expect(callsTo(apiCalls, "GET", "/v1/routes").map((call) => call.status)).toContain(200);
  expect(callsTo(apiCalls, "GET", "/v1/webui/config").map((call) => call.status)).toContain(200);
});

test("a stage's geometry and its surface reach the map", async ({ bundlePage: page, apiCalls }) => {
  await openStage(page, LOOP_STAGE.routeId, LOOP_STAGE.stageOrder);

  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Synthetic Kaiserstuhl Loop");
  await expect(page.locator(".maplibregl-canvas")).toBeVisible();
  await expect(page.locator(".stage-detail__facts")).toContainText("km");
  // The coordinates, the bounding box and the surface ranges all came out of the
  // geometry view; a map that fitted its camera and a key that lists classes are
  // the two halves of that having been read.
  await expect(mapRegion(page)).toContainText("Starts and finishes");
  await expect(page.getByRole("list", { name: "Surface classes" })).toBeVisible();

  const geometry = callsTo(
    apiCalls,
    "GET",
    `/v1/routes/${LOOP_STAGE.routeId}/stages/${LOOP_STAGE.stageOrder}/geometry`,
  );
  expect(geometry.map((call) => call.status)).toContain(200);
  await settleMap(page);
});

test("the status view drives the synchronisation panel", async ({ bundlePage: page, apiCalls }) => {
  await openLibrary(page);

  const panel = page.getByRole("region", { name: /synchronisation/i });
  await expect(panel).toBeVisible();
  // Both configured slots, the schedule state of both halves, and a run summary:
  // the whole status view, rendered.
  await expect(page.getByText("rider-b")).toBeVisible();
  await expect(page.getByRole("button", { name: "Run now: Read from VeloPlanner" })).toBeVisible();
  await expect(page.getByLabel("Schedule: Read from VeloPlanner")).toBeVisible();
  expect(callsTo(apiCalls, "GET", "/v1/status").map((call) => call.status)).toContain(200);
});

test("a stage the library does not hold is reported safely", async ({
  bundlePage: page,
  apiCalls,
  identity,
}) => {
  await page.goto(`/routes/${ABSENT_STAGE.routeId}/${ABSENT_STAGE.stageOrder}`);

  // The service answers 404 with its error envelope, and the page says what that
  // means for the reader rather than showing a code, a stack, or a blank map.
  await expect(page.getByText("No geometry for this route yet.")).toBeVisible();
  const geometry = callsTo(
    apiCalls,
    "GET",
    `/v1/routes/${ABSENT_STAGE.routeId}/stages/${ABSENT_STAGE.stageOrder}/geometry`,
  );
  expect(geometry.map((call) => call.status)).toContain(404);

  // The same request again, from outside the page, to state the shape the UI just
  // handled: an error envelope naming a code, and nothing about what went missing
  // inside the service.
  const envelope = await page.request.get(
    `/v1/routes/${ABSENT_STAGE.routeId}/stages/${ABSENT_STAGE.stageOrder}/geometry`,
    { headers: identity },
  );
  expect(envelope.status()).toBe(404);
  expect(await envelope.json()).toEqual({
    error: { code: "not_found", message: expect.any(String) },
  });
});
