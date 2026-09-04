/**
 * The wind field, which is the only thing in this application drawn by a shader.
 *
 * It has a file of its own because it is the one overlay whose whole output is
 * pixels: `windField.ts` decides where a streak goes, `windStreakLayer.ts` puts
 * it on the map, and between the two sits a projection matrix that no unit test
 * can check. The field once shipped calling every GL entry point correctly and
 * drawing nothing at all — right shader, right buffer, wrong one of the two
 * matrices MapLibre hands a custom layer — with no error, no failed compile and
 * a blank corridor. Only two frames of a real map tell that apart from a field
 * that works.
 *
 * Two readers, so two motion preferences. The suite forces `reduce` on every
 * page, which is the second test's ground; the first has to override it,
 * because a field that does not move is the bug.
 */

import type { Page } from "@playwright/test";
import { expect, mapRegion, openRoute, settleMap, test } from "./fixtures";

/** A straight three-band route, whose corridor is wide enough to hold a field. */
const LINE_ROUTE = { provider: "veloplanner", sourceRouteId: 4101, stageOrder: 3 };

/**
 * How many pixels of the map two frames of a live field must differ by.
 *
 * The field is a few dozen hairline streaks drawn faint, so the honest floor is
 * low — but what this guards against is exact: a field projected by the wrong
 * matrix is clipped away entirely, and two frames of it differ by nothing.
 */
const MOVING_PIXELS = 40;

/** A departure two hours out, stored where `useStartTime` reads it. */
async function chooseStartTime(page: Page): Promise<void> {
  const soon = new Date(Date.now() + 2 * 60 * 60 * 1000).toISOString();
  await page.addInitScript(
    (at) => window.localStorage.setItem("domestique.start-time", at as string),
    soon,
  );
}

/**
 * How many pixels two screenshots differ at, decoded in the page itself.
 *
 * A count rather than an equality, and with a threshold: a settled map drawn
 * through a software rasteriser does not read back the same bytes twice, so
 * comparing buffers reports a still picture as a moving one.
 */
async function changedPixels(page: Page, before: Buffer, after: Buffer): Promise<number> {
  return page.evaluate(
    async ([first, second]) => {
      const decode = async (base64: string) => {
        const response = await fetch(`data:image/png;base64,${base64}`);
        const bitmap = await createImageBitmap(await response.blob());
        const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
        const context = canvas.getContext("2d");
        if (!context) {
          throw new Error("no 2d context to compare frames in");
        }
        context.drawImage(bitmap, 0, 0);

        return context.getImageData(0, 0, bitmap.width, bitmap.height).data;
      };
      const one = await decode(first ?? "");
      const two = await decode(second ?? "");
      let changed = 0;
      for (let index = 0; index < one.length; index += 4) {
        const difference =
          Math.abs((one[index] ?? 0) - (two[index] ?? 0)) +
          Math.abs((one[index + 1] ?? 0) - (two[index + 1] ?? 0)) +
          Math.abs((one[index + 2] ?? 0) - (two[index + 2] ?? 0));
        // Well above the rasteriser's own frame-to-frame noise, which never
        // reaches one percent of a channel.
        if (difference > 12) {
          changed += 1;
        }
      }

      return changed;
    },
    [before.toString("base64"), after.toString("base64")],
  );
}

/** The route open on the wind measure, which is the only one the field draws for. */
async function showWind(page: Page): Promise<void> {
  await chooseStartTime(page);
  await openRoute(page, LINE_ROUTE.provider, LINE_ROUTE.sourceRouteId, LINE_ROUTE.stageOrder);
  await page
    .getByRole("group", { name: "Conditions washed along the route" })
    .getByRole("button", { name: "Wind", exact: true })
    .click();
}

test.describe("a reader who has not refused movement", () => {
  test.use({ contextOptions: { reducedMotion: "no-preference" } });

  test("sees the field drift across the corridor", async ({ offlinePage: page }) => {
    const region = mapRegion(page);
    await showWind(page);
    // The forecast has to arrive and the field to be seeded before there is
    // anything to compare; the map itself is settled by `openRoute`.
    await page.waitForTimeout(3000);

    const first = await region.screenshot();
    await page.waitForTimeout(500);

    expect(await changedPixels(page, first, await region.screenshot())).toBeGreaterThan(
      MOVING_PIXELS,
    );
  });
});

test.describe("a reader who has refused it", () => {
  test("gets arrows in the corridor, and a map that stays still", async ({ offlinePage: page }) => {
    const region = mapRegion(page);
    await showWind(page);

    await expect(page.locator(".route-wind-arrow-marker").first()).toBeVisible();
    const still = await settleMap(page);
    await page.waitForTimeout(500);

    expect(await changedPixels(page, still, await region.screenshot())).toBe(0);
  });
});
