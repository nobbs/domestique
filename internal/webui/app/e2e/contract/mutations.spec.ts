/**
 * The requests that change something, through the gates that guard them.
 *
 * Each one is a state-changing route, so each one has to satisfy the identity
 * gate and the origin check before it reaches a handler at all — and unlike a
 * handler test, the request here is composed by the shipped client and sent by a
 * real browser. What the service stored afterwards is read back the way a reader
 * would: from the page.
 */

import { openRoute, openSync } from "../fixtures";
import { callsTo, expect, test } from "./fixtures";

const LINE_ROUTE = { routeId: 4101, stageOrder: 1 };
const SOURCE_SWITCH = "Hourly: Read from VeloPlanner";

test("changing the schedule is stored and read back", async ({ bundlePage: page, apiCalls }) => {
  await openSync(page);

  const switchControl = page.getByLabel(SOURCE_SWITCH);
  await expect(switchControl).toBeVisible();
  const wasScheduled = await switchControl.isChecked();

  await switchControl.click();

  // The service answers with what it stored, and the client parses that answer
  // rather than assuming the request went through.
  await expect(switchControl).toBeChecked({ checked: !wasScheduled });
  const put = callsTo(apiCalls, "PUT", "/v1/sync/schedule");
  expect(put.map((call) => call.status)).toEqual([200]);

  // A reload proves the change reached SQLite instead of only the page's cache.
  await page.reload();
  await expect(page.getByLabel(SOURCE_SWITCH)).toBeChecked({ checked: !wasScheduled });

  await page.getByLabel(SOURCE_SWITCH).click();
  await expect(page.getByLabel(SOURCE_SWITCH)).toBeChecked({ checked: wasScheduled });
});

test("asking for a run now is accepted", async ({ bundlePage: page, apiCalls }) => {
  await openSync(page);

  await page.getByRole("button", { name: "Run now: Read from VeloPlanner" }).click();

  // 202 and no body: the client is told the work was taken on, not that it
  // finished. The demo's run re-seeds the synthetic library, so nothing is
  // fetched from anywhere and the run it reports is a real one.
  await expect(async () => {
    expect(callsTo(apiCalls, "POST", "/v1/sync/source").map((call) => call.status)).toEqual([202]);
  }).toPass();
  // The card is still the one the status view drives, and it did not fall into an
  // error state over a response with nothing in it to parse.
  await expect(page.locator(".sync-card__error")).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Now" })).toBeVisible();
});

test("asking for one route to be redone is accepted", async ({ bundlePage: page, apiCalls }) => {
  await openRoute(page, LINE_ROUTE.routeId, LINE_ROUTE.stageOrder);

  await page.getByRole("button", { name: "Reprocess" }).click();

  await expect(page.getByText(/Queued\./)).toBeVisible();
  expect(
    callsTo(
      apiCalls,
      "POST",
      `/v1/routes/${LINE_ROUTE.routeId}/stages/${LINE_ROUTE.stageOrder}/reprocess`,
    ).map((call) => call.status),
  ).toEqual([202]);
});

test("a state-changing request without the configured origin is refused", async ({
  bundlePage: page,
  identity,
}) => {
  // The gate the browser satisfies implicitly, asserted explicitly: the same
  // identity, the same route, a different origin. This is the one request in the
  // suite that is meant to fail, and it fails at the provenance check rather than
  // at the handler.
  const refused = await page.request.post("/v1/sync/source", {
    headers: { ...identity, origin: "https://elsewhere.example.test" },
  });

  expect(refused.status()).toBe(403);
  expect(await refused.json()).toEqual({
    error: { code: "forbidden", message: expect.any(String) },
  });
});
