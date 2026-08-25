/**
 * Choosing the ground the library is drawn on.
 *
 * The chooser is the one control on this page whose effect is a network
 * request: everything else it could be asserted about — that a radio is
 * checked, that a name is announced — a jsdom test already covers. What only a
 * browser can show is that pressing it makes MapLibre fetch the other
 * provider's style document, that the credit under it changes hands, and that
 * the pick outlives the page it was made on.
 *
 * The demo offers several public cartographies. The harness also adds one
 * isolated style, so this test can prove that a choice changes both the style
 * request and the attribution without reaching the public tile provider.
 */

import {
  BASEMAP_ATTRIBUTION_TEXT,
  SECOND_BASEMAP_ATTRIBUTION_TEXT,
  SECOND_BASEMAP_MARKER,
  SECOND_BASEMAP_NAME,
} from "./basemap";
import { expect, installOfflineBasemap, openLibrary, pinRendering, test } from "./fixtures";

/** The style documents the page asked for, marker aside. */
function askedForSecond(requested: string[]): boolean {
  return requested.some((url) => url.includes(SECOND_BASEMAP_MARKER));
}

test.describe("the basemap chooser", () => {
  test("switches the map to the other provider, credit and all", async ({ page, baseURL }) => {
    const requested: string[] = [];
    const leaks = await installOfflineBasemap(page, baseURL ?? "", {
      requested,
      secondBasemap: true,
    });
    await pinRendering(page);
    await openLibrary(page);

    // What a reader who has chosen nothing gets: the first configured entry.
    expect(askedForSecond(requested)).toBe(false);
    await page.getByRole("button", { name: "Show the map credit" }).click();
    await expect(page.getByText(BASEMAP_ATTRIBUTION_TEXT)).toBeVisible();

    await page.getByRole("button", { name: "Choose the basemap" }).click();
    await page.getByRole("radio", { name: SECOND_BASEMAP_NAME }).check();

    // The request is the assertion: a canvas cannot be read back, so what proves
    // the map changed hands is which document MapLibre went and fetched.
    await expect.poll(() => askedForSecond(requested)).toBe(true);
    // And the credit followed it, because a provider's terms are its own.
    await expect(page.getByText(SECOND_BASEMAP_ATTRIBUTION_TEXT)).toBeVisible();

    expect(leaks, "no request left the page for a third-party server").toEqual([]);
  });

  test("remembers the pick for the next visit", async ({ page, baseURL }) => {
    const requested: string[] = [];
    const leaks = await installOfflineBasemap(page, baseURL ?? "", {
      requested,
      secondBasemap: true,
    });
    await pinRendering(page);
    await openLibrary(page);

    await page.getByRole("button", { name: "Choose the basemap" }).click();
    await page.getByRole("radio", { name: SECOND_BASEMAP_NAME }).check();
    await expect.poll(() => askedForSecond(requested)).toBe(true);

    requested.length = 0;
    await page.reload();
    await openLibrary(page);

    // Not merely that it was asked for again, but that it was the only thing
    // asked for: a page that loaded the first entry and then corrected itself
    // would flash the wrong ground and fetch two documents to draw one map.
    await expect.poll(() => requested.length).toBeGreaterThan(0);
    expect(requested.every((url) => url.includes(SECOND_BASEMAP_MARKER))).toBe(true);
    await expect(page.getByRole("button", { name: "Choose the basemap" })).toBeVisible();

    expect(leaks, "no request left the page for a third-party server").toEqual([]);
  });
});
