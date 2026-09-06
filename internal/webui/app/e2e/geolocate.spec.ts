/**
 * The locate button: jumping the camera to where the reader is.
 *
 * jsdom has no Geolocation API and no camera to fly, so what proves the button
 * works — and what proves the library asks once as it opens, a deep link never
 * does, and a denial is honest — needs a real browser. The camera's own state cannot be read back any
 * more than the basemap chooser's can, so a screenshot is the evidence, the same
 * way it is in `basemap.spec.ts` — cropped away from the corners, because the
 * canvas fills the whole map behind the controls and a denial's own icon change
 * would otherwise be mistaken for the camera having moved.
 */

import type { Page } from "@playwright/test";
import { expect, openLibrary, openRoute, settleMap, test } from "./fixtures";

// The demo library sits near 48.40N 8.10E; Paris is nowhere close, so a jump
// to it reads unmistakably in a screenshot diff.
const READER_POSITION = { latitude: 48.8566, longitude: 2.3522 };

/** A route the demo library holds, for a link that lands on it. */
const LOOP_ROUTE = { provider: "veloplanner", sourceRouteId: 4102, stageOrder: 1 };

function locateButton(page: Page) {
  return page.getByRole("button", { name: "Find my location" });
}

/**
 * What the camera actually painted, independent of the controls drawn over it.
 *
 * The canvas element fills the whole map, including the controls — an element
 * screenshot is the composited page, not a read of the WebGL buffer alone — so
 * this crops to its centre.
 */
async function cameraScreenshot(page: Page): Promise<Buffer> {
  const box = await page.locator(".maplibregl-canvas").boundingBox();
  if (!box) {
    throw new Error("the map canvas has no bounding box");
  }

  return page.screenshot({
    clip: {
      x: box.x + box.width * 0.2,
      y: box.y + box.height * 0.2,
      width: box.width * 0.6,
      height: box.height * 0.6,
    },
  });
}

/** Records every `getCurrentPosition`/`watchPosition` call the page makes, in order. */
async function trackGeolocationCalls(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const calls: string[] = [];
    (window as unknown as { __geoCalls: string[] }).__geoCalls = calls;
    const real = navigator.geolocation;
    for (const name of ["getCurrentPosition", "watchPosition"] as const) {
      const original = real[name].bind(real);
      // @ts-expect-error monkey-patching a DOM API for the harness alone
      real[name] = (...args: unknown[]) => {
        calls.push(name);
        // @ts-expect-error see above
        return original(...args);
      };
    }
  });
}

function geolocationCalls(page: Page): Promise<string[]> {
  return page.evaluate(() => (window as unknown as { __geoCalls: string[] }).__geoCalls);
}

test.describe("granted", () => {
  test.use({ permissions: ["geolocation"], geolocation: READER_POSITION });

  test("jumps the camera to the granted position", async ({ offlinePage: page }) => {
    await openLibrary(page);
    const before = await cameraScreenshot(page);

    await locateButton(page).click();
    await settleMap(page);

    expect((await cameraScreenshot(page)).equals(before)).toBe(false);
    await expect(page.getByRole("img", { name: "Your location" })).toBeVisible();
  });

  test("answers the keyboard the same way it answers a click", async ({ offlinePage: page }) => {
    await openLibrary(page);
    const before = await cameraScreenshot(page);

    await locateButton(page).focus();
    await page.keyboard.press("Enter");
    await settleMap(page);

    expect((await cameraScreenshot(page)).equals(before)).toBe(false);
  });

  // `trackUserLocation` is left at its default, so a press must resolve one
  // position rather than open a continuous watch. Chromium's StrictMode
  // double-mount can ask `getCurrentPosition` twice in the dev build; what must
  // never appear is `watchPosition`.
  test("asks for one-shot positions, never a continuous watch", async ({ offlinePage: page }) => {
    await trackGeolocationCalls(page);
    await openLibrary(page);

    await locateButton(page).click();
    await settleMap(page);

    const calls = await geolocationCalls(page);
    expect(calls.length).toBeGreaterThan(0);
    expect(calls.every((call) => call === "getCurrentPosition")).toBe(true);
  });
});

test("asks for the reader's position as the library opens, and reads it only", async ({
  offlinePage: page,
}) => {
  await trackGeolocationCalls(page);

  await openLibrary(page);

  const calls = await geolocationCalls(page);
  expect(calls.length).toBeGreaterThan(0);
  expect(calls.every((call) => call === "getCurrentPosition")).toBe(true);
});

test("asks for no position when a route is deep-linked", async ({ offlinePage: page }) => {
  await trackGeolocationCalls(page);

  await openRoute(page, LOOP_ROUTE.provider, LOOP_ROUTE.sourceRouteId, LOOP_ROUTE.stageOrder);

  expect(await geolocationCalls(page)).toEqual([]);
});

test("denying permission leaves the camera untouched and raises no error", async ({
  offlinePage: page,
}) => {
  await openLibrary(page);
  const before = await cameraScreenshot(page);

  // No permission was granted, so Chromium answers the request as denied
  // without a dialog — the same answer a reader who says no gets.
  await locateButton(page).click();
  await settleMap(page);

  expect((await cameraScreenshot(page)).equals(before)).toBe(true);
  await expect(page.getByRole("alert")).toHaveCount(0);
});
