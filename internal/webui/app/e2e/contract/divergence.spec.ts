/**
 * What a drift between the Go views and this client actually looks like.
 *
 * The rest of this project asserts that the two agree. This one asserts what
 * happens when they do not, because a contract check that fails as "the page is
 * empty" tells nobody which endpoint changed. A response is deliberately given a
 * field the client does not expect, and the page is required to name both the
 * request it came back from and the field that did not fit.
 */

import { expect, test } from "./fixtures";

test("a diverged response names the endpoint and the field", async ({ bundlePage: page }) => {
  // Registered after the harness's own handler, so this one wins for this URL:
  // the shape below never came from the service, it stands in for a view whose
  // `route_id` stopped being a number.
  await page.route("**/v1/routes", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        stages: [
          {
            route_id: "4101",
            stage: 1,
            title: "Synthetic Rhine Traverse — Valley floor",
            route_name: "Synthetic Rhine Traverse",
            stage_name: "Valley floor",
            distance_metres: 12_600,
            point_count: 120,
          },
        ],
      }),
    });
  });

  await page.goto("/");

  const failure = page.getByText(/unexpected API response/);
  await expect(failure).toBeVisible();
  // Both halves of the diagnostic: the request, which only the client layer knows,
  // and the field, which only the parser knows.
  await expect(failure).toContainText("GET /v1/routes");
  await expect(failure).toContainText("stages[0].route_id");
  await expect(failure).toContainText("is not a finite number");
});
