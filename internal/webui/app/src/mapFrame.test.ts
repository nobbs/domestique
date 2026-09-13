/**
 * A map's canvas must be exactly the box it is drawn in: MapLibre frames a
 * route against the canvas, so any part of it clipped away pushes the route
 * off-centre or out of sight.
 */

import { readFileSync } from "node:fs";
import { expect, it } from "vitest";

// Vitest runs with the app directory as its root, which anchors this path.
const css = readFileSync("src/index.css", "utf8");

it("gives no map a height of its own in any media block", () => {
  const sizedMaps = [...css.matchAll(/\.(?:route-map|maplibregl-map)[^{]*\{[^}]*\bmin-height\b/g)];

  expect(sizedMaps.map((match) => match[0])).toEqual([]);
});
