/**
 * The sync page: the three cards, the notice a notification lands on, and the
 * line that says which build is running.
 *
 * What is asserted here needs a whole page rather than a component: the cards
 * are driven by two separate queries against the running service, the notice
 * resolves a reference out of the recorded history, and the way in and out is a
 * URL — a query parameter arriving from a Pushover message, and a link back to
 * the map.
 */

import { expect, openLibrary, openSync, test } from "./fixtures";

/** A reference no recorded run can have, standing in for a pruned one. */
const PRUNED = "000000000000";

test("the page answers the three questions in order", async ({ offlinePage: page }) => {
  await openSync(page);

  const headings = page.getByRole("heading", { level: 2 });
  await expect(headings).toHaveText(["Now", "What the accounts hold", "What has happened"]);
  // Each card really read its view: the schedule and the run control come from
  // the status, the slots from the same status, and the rows from the history.
  await expect(page.getByRole("button", { name: "Run now: Read from VeloPlanner" })).toBeVisible();
  await expect(page.getByText("rider-b", { exact: true })).toBeVisible();
  await expect(page.getByText(/not what a head unit has downloaded/)).toBeVisible();
  await expect(page.getByRole("region", { name: "What has happened" })).toContainText(/\d/);
});

test("the way back is the map itself", async ({ offlinePage: page }) => {
  await openSync(page);

  await page.getByRole("link", { name: /Back to the map/ }).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("searchbox", { name: "Search the route library" })).toBeVisible();
});

test("a notification about a run that is gone says so", async ({ offlinePage: page }) => {
  await page.goto(`/sync?run=${PRUNED}`);

  // The notice is the first card, above the three: an operator who followed a
  // notification is told what became of the run it named before they are told
  // anything else.
  const notice = page.locator(".run-notice");
  await expect(notice).toBeVisible();
  await expect(notice).toContainText("That run is no longer kept");
  await expect(notice).toContainText(PRUNED);
  await expect(page.getByRole("heading", { level: 1, name: "Sync" })).toBeVisible();
});

test("a notification about a recorded run lands on that run", async ({ offlinePage: page }) => {
  await openSync(page);

  // Whatever the demo last recorded, taken from the history rather than assumed:
  // the assertion is that a reference read off a row resolves back to a notice
  // about that same row.
  const reference = await page.locator(".run-row__reference").first().innerText();
  expect(reference).not.toBe("");

  await page.goto(`/sync?run=${reference.trim()}`);

  const notice = page.locator(".run-notice");
  await expect(notice).toBeVisible();
  await expect(notice).toContainText(reference.trim());
  await expect(notice).not.toContainText("no longer kept");
});

test("the foot of the page names the running build", async ({ offlinePage: page }) => {
  await openSync(page);

  const build = page.locator(".sync-page__build a");
  await expect(build).toBeVisible();
  // The demo is a local build, so the link is the repository and the words say
  // which kind of process this is rather than printing a commit it does not have.
  await expect(build).toHaveAttribute("href", /github\.com\/nobbs\/domestique/);
  await expect(build).toHaveAttribute("rel", /noreferrer/);
  await expect(build).toHaveAttribute("target", "_blank");
});

test("the map's wordmark is the only way in that a reader needs", async ({ offlinePage: page }) => {
  await openLibrary(page);

  await page.getByRole("link", { name: /^Sync/ }).click();

  await expect(page).toHaveURL(/\/sync$/);
  await expect(page.getByRole("region", { name: "Now" })).toBeVisible();
});
