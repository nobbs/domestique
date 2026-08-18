import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import {
  gapsOutside,
  highlightLabel,
  highlightRanges,
  intersectRanges,
  litRanges,
  sameHighlight,
} from "./highlight";

/** Points spaced evenly by latitude, so every stretch is the same length. */
function route(pointCount: number): Position[] {
  return Array.from({ length: pointCount }, (_, index) => [8, 49 + index * 0.001] as Position);
}

/** One ten-thousandth of a degree of latitude is 11.119 metres. */
const FINE_SPACING_METRES = 11.119;

/** A route whose segments run at the given gradients, in percent. */
function ramp(percents: number[]): Position[] {
  const points: Position[] = [[8, 49, 100]];
  percents.forEach((percent, index) => {
    const previous = points[index] as [number, number, number];
    points.push([
      8,
      49 + (index + 1) * 0.0001,
      previous[2] + (FINE_SPACING_METRES * percent) / 100,
    ]);
  });

  return points;
}

describe("sameHighlight", () => {
  it("tells one class from another of the same kind", () => {
    expect(sameHighlight({ type: "band", band: 2 }, { type: "band", band: 2 })).toBe(true);
    expect(sameHighlight({ type: "band", band: 2 }, { type: "band", band: 3 })).toBe(false);
  });

  // A band and a surface class are never the same selection, whatever their
  // numbering happens to look like.
  it("tells a band from a surface class", () => {
    expect(sameHighlight({ type: "band", band: 0 }, { type: "surface", kind: "asphalt" })).toBe(
      false,
    );
  });

  it("counts nothing picked as the same as nothing picked", () => {
    expect(sameHighlight(null, null)).toBe(true);
    expect(sameHighlight(null, { type: "surface", kind: "gravel" })).toBe(false);
  });
});

describe("highlightLabel", () => {
  it("names a class the way the palette that draws it does", () => {
    expect(highlightLabel({ type: "surface", kind: "gravel" })).toBe("Gravel");
    expect(highlightLabel({ type: "band", band: 4 })).toBe("≥ 16%");
  });
});

describe("gapsOutside", () => {
  it("returns the ground between the lit stretches, and the ends", () => {
    expect(
      gapsOutside(
        [
          { startMetres: 100, endMetres: 200 },
          { startMetres: 400, endMetres: 500 },
        ],
        0,
        800,
      ),
    ).toEqual([
      { startMetres: 0, endMetres: 100 },
      { startMetres: 200, endMetres: 400 },
      { startMetres: 500, endMetres: 800 },
    ]);
  });

  it("veils the whole stretch when nothing in it is lit", () => {
    expect(gapsOutside([], 0, 800)).toEqual([{ startMetres: 0, endMetres: 800 }]);
  });

  it("veils nothing when the lit stretch covers everything", () => {
    expect(gapsOutside([{ startMetres: 0, endMetres: 800 }], 0, 800)).toEqual([]);
  });

  // A window shows part of the stage; the stretches either side of it are not
  // gaps in the chart, they are simply not on it.
  it("stays inside the stretch on show", () => {
    expect(gapsOutside([{ startMetres: 0, endMetres: 100 }], 200, 400)).toEqual([
      { startMetres: 200, endMetres: 400 },
    ]);
  });

  it("absorbs touching stretches rather than reporting a gap of no width", () => {
    expect(
      gapsOutside(
        [
          { startMetres: 0, endMetres: 400 },
          { startMetres: 400, endMetres: 800 },
        ],
        0,
        800,
      ),
    ).toEqual([]);
  });
});

describe("highlightRanges", () => {
  it("finds every stretch of a surface class, wherever it falls", () => {
    const ranges = highlightRanges(
      route(9),
      [
        { kind: "asphalt", startIndex: 0, endIndex: 1 },
        { kind: "gravel", startIndex: 2, endIndex: 3 },
        { kind: "asphalt", startIndex: 4, endIndex: 5 },
        { kind: "gravel", startIndex: 6, endIndex: 7 },
      ],
      { type: "surface", kind: "gravel" },
    );

    // One point past the last segment, which is where the class hands over.
    expect(ranges).toEqual([
      { startIndex: 2, endIndex: 4 },
      { startIndex: 6, endIndex: 8 },
    ]);
  });

  it("finds the stretches of one gradient band", () => {
    const steps = ramp([...Array(40).fill(0), ...Array(40).fill(14)]);
    const ranges = highlightRanges(steps, [], { type: "band", band: 3 });

    expect(ranges).not.toHaveLength(0);
    expect(ranges.every((range) => range.endIndex > range.startIndex)).toBe(true);
  });

  // Asking for ground the stage has none of lights nothing, which is the honest
  // answer rather than a fallback to the whole route.
  it("finds nothing when the stage has none of the class", () => {
    expect(
      highlightRanges(route(5), [{ kind: "asphalt", startIndex: 0, endIndex: 3 }], {
        type: "surface",
        kind: "gravel",
      }),
    ).toEqual([]);
  });

  it("finds nothing in geometry too short to span any ground", () => {
    expect(
      highlightRanges(route(1), [{ kind: "gravel", startIndex: 0, endIndex: 0 }], {
        type: "surface",
        kind: "gravel",
      }),
    ).toEqual([]);
  });
});

describe("intersectRanges", () => {
  it("keeps only the ground both restrictions agree on", () => {
    expect(
      intersectRanges(
        [
          { startIndex: 0, endIndex: 10 },
          { startIndex: 20, endIndex: 30 },
        ],
        [{ startIndex: 5, endIndex: 25 }],
      ),
    ).toEqual([
      { startIndex: 5, endIndex: 10 },
      { startIndex: 20, endIndex: 25 },
    ]);
  });

  it("drops an overlap of a single point, which spans no ground", () => {
    expect(
      intersectRanges([{ startIndex: 0, endIndex: 5 }], [{ startIndex: 5, endIndex: 9 }]),
    ).toEqual([]);
  });
});

describe("litRanges", () => {
  it("dims nothing when nothing has been asked of the route", () => {
    expect(litRanges(null, null)).toBeNull();
  });

  it("lights the window when only the chart is zoomed", () => {
    expect(litRanges({ startIndex: 2, endIndex: 6 }, null)).toEqual([
      { startIndex: 2, endIndex: 6 },
    ]);
  });

  it("lights the class when the chart is showing the whole route", () => {
    expect(litRanges(null, [{ startIndex: 2, endIndex: 6 }])).toEqual([
      { startIndex: 2, endIndex: 6 },
    ]);
  });

  // A reader looking at two kilometres of route who asks for gravel means the
  // gravel in those two kilometres.
  it("narrows a picked class to the stretch on show", () => {
    expect(
      litRanges({ startIndex: 4, endIndex: 12 }, [
        { startIndex: 0, endIndex: 6 },
        { startIndex: 10, endIndex: 20 },
      ]),
    ).toEqual([
      { startIndex: 4, endIndex: 6 },
      { startIndex: 10, endIndex: 12 },
    ]);
  });
});
