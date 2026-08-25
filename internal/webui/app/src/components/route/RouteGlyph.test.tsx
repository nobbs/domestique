import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Position } from "../../api/types";
import { glyphPoints, RouteGlyph } from "./RouteGlyph";

function parse(points: string): Array<[number, number]> {
  return points.split(" ").map((pair) => pair.split(",").map(Number) as [number, number]);
}

describe("glyphPoints", () => {
  it("fits the shape inside the padded viewBox", () => {
    const square: Position[] = [
      [8.0, 49.0],
      [8.1, 49.0],
      [8.1, 49.1],
      [8.0, 49.1],
      [8.0, 49.0],
    ];

    const parsed = parse(glyphPoints(square));
    const xs = parsed.map(([x]) => x);
    const ys = parsed.map(([, y]) => y);

    expect(Math.min(...xs)).toBeGreaterThanOrEqual(0);
    expect(Math.max(...xs)).toBeLessThanOrEqual(48);
    expect(Math.min(...ys)).toBeGreaterThanOrEqual(0);
    expect(Math.max(...ys)).toBeLessThanOrEqual(48);
  });

  it("puts north at the top", () => {
    const northward: Position[] = [
      [8.0, 49.0],
      [8.0, 49.5],
    ];

    const [south, north] = parse(glyphPoints(northward));

    // SVG y grows downwards, so the northern point must have the smaller y.
    expect(north?.[1]).toBeLessThan(south?.[1] ?? 0);
  });

  it("preserves aspect ratio rather than stretching to fill", () => {
    // A shape twice as wide as it is tall must stay twice as wide.
    const wide: Position[] = [
      [8.0, 49.0],
      [8.2, 49.0],
      [8.2, 49.05],
      [8.0, 49.05],
      [8.0, 49.0],
    ];

    const parsed = parse(glyphPoints(wide));
    const width = Math.max(...parsed.map(([x]) => x)) - Math.min(...parsed.map(([x]) => x));
    const height = Math.max(...parsed.map(([, y]) => y)) - Math.min(...parsed.map(([, y]) => y));

    expect(width).toBeGreaterThan(height);
  });

  it("returns nothing for geometry too short to draw", () => {
    expect(glyphPoints([])).toBe("");
    expect(glyphPoints([[8, 49]])).toBe("");
  });

  it("does not produce NaN for a degenerate repeated point", () => {
    const points = glyphPoints([
      [8, 49],
      [8, 49],
    ]);

    expect(points).not.toContain("NaN");
  });
});

describe("RouteGlyph", () => {
  it("labels the shape for assistive technology", () => {
    render(
      <RouteGlyph
        coordinates={[
          [8.0, 49.0],
          [8.1, 49.1],
        ]}
        title="Eich Rundkurs 90"
        band={2}
      />,
    );

    expect(screen.getByRole("img", { name: /Eich Rundkurs 90/ })).toBeInTheDocument();
  });

  it("renders a presentational placeholder instead of an empty graphic", () => {
    render(<RouteGlyph coordinates={[]} title="Empty" band={0} />);

    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByRole("presentation")).toBeInTheDocument();
  });
});

describe("RouteGlyph", () => {
  it("carries the band as data, for the ramp in the stylesheet to colour", () => {
    const { container } = render(
      <RouteGlyph
        coordinates={[
          [8.0, 49.0],
          [8.1, 49.1],
        ]}
        title="Steep one"
        band={4}
      />,
    );

    expect(container.querySelector("polyline")?.getAttribute("data-band")).toBe("4");
  });
});
