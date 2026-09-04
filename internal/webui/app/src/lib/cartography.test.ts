/**
 * Holds the TypeScript cartography colours to the CSS custom properties: the
 * two sides cannot share a copy at runtime (see cartography.ts), so this
 * parses index.css and fails the moment a hex is changed on one side only.
 */

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { SURFACE_KINDS } from "../api/types";
import { INK, PANEL, ROUTE_ACCENT } from "./cartography";
import {
  cloudColour,
  rainColour,
  temperatureHexColour,
  windColour,
  windRelationColour,
} from "./measures";
import { bandColour } from "./profile";
import { surfaceColour } from "./surface";

// Vitest runs with the app directory as its root, which anchors this path.
const css = readFileSync("src/index.css", "utf8");

/** The declarations of one top-level block, by custom-property name. */
function varsIn(selector: string): Map<string, string> {
  const start = css.indexOf(`${selector} {`);
  expect(start, `index.css no longer has a "${selector}" block`).toBeGreaterThanOrEqual(0);
  const block = css.slice(start, css.indexOf("}", start));

  return new Map(
    [...block.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((match) => [
      match[1] as string,
      (match[2] as string).trim(),
    ]),
  );
}

const light = varsIn(":root");
// The explicit-choice and system-preference blocks carry the same palette;
// both are checked so a hex changed in only one of them is caught too.
const darkBlocks = [
  { name: '[data-theme="dark"]', vars: varsIn(':root[data-theme="dark"]') },
  {
    name: "prefers-color-scheme",
    vars: varsIn(':root:not([data-theme="light"])'),
  },
];

/** The TS kind `unknown` is `--surface-unsurveyed` in the CSS. */
function surfaceVariable(kind: string): string {
  return `--surface-${kind === "unknown" ? "unsurveyed" : kind}`;
}

describe("cartography tokens", () => {
  it("matches the light palette", () => {
    expect(light.get("--accent")).toBe(ROUTE_ACCENT.light);
    expect(light.get("--panel")).toBe(PANEL.light);
    expect(light.get("--ink")).toBe(INK.light);
    for (const band of [0, 1, 2, 3, 4]) {
      expect(light.get(`--grade-${band}`)).toBe(bandColour(band, false));
      expect(light.get(`--temp-${band}`)).toBe(temperatureHexColour(band, false));
    }
    for (const band of [0, 1, 2, 3]) {
      expect(light.get(`--wind-${band}`)).toBe(windColour(band, false));
      expect(light.get(`--wind-relation-${band}`)).toBe(windRelationColour(band, false));
      expect(light.get(`--rain-${band}`)).toBe(rainColour(band, false));
      expect(light.get(`--cloud-${band}`)).toBe(cloudColour(band, false));
    }
    expect(light.get("--wind-relation-mixed")).toBe(windRelationColour(null, false));
    for (const kind of SURFACE_KINDS) {
      expect(light.get(surfaceVariable(kind))).toBe(surfaceColour(kind, false));
    }
  });

  it.each(darkBlocks)("matches the dark palette in $name", ({ vars }) => {
    expect(vars.get("--accent")).toBe(ROUTE_ACCENT.dark);
    expect(vars.get("--panel")).toBe(PANEL.dark);
    expect(vars.get("--ink")).toBe(INK.dark);
    for (const band of [0, 1, 2, 3, 4]) {
      expect(vars.get(`--grade-${band}`)).toBe(bandColour(band, true));
      expect(vars.get(`--temp-${band}`)).toBe(temperatureHexColour(band, true));
    }
    for (const band of [0, 1, 2, 3]) {
      expect(vars.get(`--wind-${band}`)).toBe(windColour(band, true));
      expect(vars.get(`--wind-relation-${band}`)).toBe(windRelationColour(band, true));
      expect(vars.get(`--rain-${band}`)).toBe(rainColour(band, true));
      expect(vars.get(`--cloud-${band}`)).toBe(cloudColour(band, true));
    }
    expect(vars.get("--wind-relation-mixed")).toBe(windRelationColour(null, true));
    for (const kind of SURFACE_KINDS) {
      expect(vars.get(surfaceVariable(kind))).toBe(surfaceColour(kind, true));
    }
  });
});
