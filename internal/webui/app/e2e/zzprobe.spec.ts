/**
 * TEMPORARY. Not a test of the application — a probe that runs where the
 * failure happens, because it does not happen on a developer machine.
 *
 * `settleMap` compares consecutive screenshots of `.route-map` and gives up
 * when they never match. This asks the two questions that separates a loop
 * from noise: how many distinct images the region produces over the same
 * window, and whether the layout under it is moving while they differ.
 */

import { createHash } from "node:crypto";
import { expect, openWorkspace, test } from "./fixtures";

test("probe: what the map region does after a route opens", async ({
  offlinePage: page,
}, testInfo) => {
  await page.goto("/?route=veloplanner%2F4102%2F1");
  await openWorkspace(page);
  await expect(page.getByRole("button", { name: /^Search \d+ routes?$/ })).toBeVisible();

  const region = page.locator(".route-map");
  await expect(region).toBeVisible();

  const digests: string[] = [];
  const layouts: string[] = [];
  // One copy of each distinct image, so the difference between the two the
  // region keeps alternating between can actually be looked at.
  const kept = new Map<string, Buffer>();
  for (let i = 0; i < 24; i += 1) {
    await page.waitForTimeout(250);
    const shot = await region.screenshot();
    const digest = createHash("sha1").update(shot).digest("hex").slice(0, 8);
    if (!kept.has(digest)) {
      kept.set(digest, shot);
    }
    digests.push(digest);
    layouts.push(
      await page.evaluate(() => {
        const aside = document.querySelector("aside");
        const map = document.querySelector(".route-map");
        const canvas = document.querySelector(".maplibregl-canvas") as HTMLCanvasElement | null;
        const a = aside?.getBoundingClientRect();
        const m = map?.getBoundingClientRect();

        return [
          `vh=${window.innerHeight}`,
          `aside=${a ? `${a.width.toFixed(1)}x${a.height.toFixed(1)}` : "-"}`,
          `bar=${aside ? aside.offsetWidth - aside.clientWidth : "-"}`,
          `map=${m ? `${m.width.toFixed(1)}x${m.height.toFixed(1)}` : "-"}`,
          `canvas=${canvas ? `${canvas.width}x${canvas.height}` : "-"}`,
        ].join(" ");
      }),
    );
  }

  // `process.stdout` rather than `console`, which the linter forbids and is
  // right to: this is a probe reporting to a CI log, not logging left behind.
  process.stdout.write(`PROBE distinct=${new Set(digests).size}/${digests.length}\n`);
  process.stdout.write(`PROBE frames=${digests.join(" ")}\n`);
  for (const [i, l] of layouts.entries()) {
    process.stdout.write(`PROBE t${String(i).padStart(2, "0")} ${l}\n`);
  }
  for (const [digest, body] of kept) {
    await testInfo.attach(`region-${digest}`, { body, contentType: "image/png" });
  }
});
