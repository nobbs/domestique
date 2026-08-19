import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { cumulativeMetres, haversineMetres } from "./profile";
import {
  bearingBetween,
  CHEVRON_SPACING_PIXELS,
  compassPoint,
  cuesDescription,
  directionChevrons,
  MAX_CHEVRONS,
  metresPerPixel,
  routeCues,
} from "./routeCues";

/** A straight eastward line of `count` points, `stepMetres` apart at latitude 49. */
function eastward(count: number, stepMetres: number): Position[] {
  const degreesPerMetre = 1 / (111_320 * Math.cos((49 * Math.PI) / 180));

  return Array.from(
    { length: count },
    (_, index): Position => [8 + index * stepMetres * degreesPerMetre, 49],
  );
}

/** A closed square starting at the south-west corner, ridden anticlockwise. */
function square(sideMetres: number): Position[] {
  const latitudeStep = sideMetres / 111_320;
  const longitudeStep = sideMetres / (111_320 * Math.cos((49 * Math.PI) / 180));
  const corners: Position[] = [
    [8, 49],
    [8 + longitudeStep, 49],
    [8 + longitudeStep, 49 + latitudeStep],
    [8, 49 + latitudeStep],
    [8, 49],
  ];

  // Densified at fifty metres, because a real stage stores points along its
  // edges rather than only where it turns, and a chevron has to land on the edge
  // it is drawn for.
  const steps = Math.round(sideMetres / 50);
  const dense: Position[] = [];
  for (let index = 0; index < corners.length - 1; index++) {
    const from = corners[index] as Position;
    const to = corners[index + 1] as Position;
    for (let step = 0; step < steps; step++) {
      dense.push([
        from[0] + ((to[0] - from[0]) * step) / steps,
        from[1] + ((to[1] - from[1]) * step) / steps,
      ]);
    }
  }
  dense.push(corners[0] as Position);

  return dense;
}

/** An out-and-back: out to the east, then back over the same ground. */
function outAndBack(halfMetres: number): Position[] {
  const out = eastward(21, halfMetres / 20);

  return [...out, ...out.slice(0, -1).reverse()];
}

describe("bearings", () => {
  it("reads the four cardinal directions", () => {
    expect(bearingBetween([8, 49], [8, 49.1])).toBeCloseTo(0, 1);
    expect(bearingBetween([8, 49], [8.1, 49])).toBeCloseTo(90, 1);
    expect(bearingBetween([8, 49], [8, 48.9])).toBeCloseTo(180, 1);
    expect(bearingBetween([8, 49], [7.9, 49])).toBeCloseTo(270, 1);
  });

  it("names a bearing in the eight points a sentence can carry", () => {
    expect(compassPoint(0)).toBe("north");
    expect(compassPoint(44)).toBe("north-east");
    expect(compassPoint(180)).toBe("south");
    expect(compassPoint(315)).toBe("north-west");
    // Wrapping either way, so a reversed bearing needs no arithmetic at the call.
    expect(compassPoint(360)).toBe("north");
    expect(compassPoint(-90)).toBe("west");
    expect(compassPoint(450)).toBe("east");
  });
});

describe("route cues", () => {
  it("reads the two ends and the way the ride leaves", () => {
    const cues = routeCues(eastward(21, 100));

    expect(cues).not.toBeNull();
    expect(cues?.start).toEqual([8, 49]);
    expect(cues?.sharedTerminal).toBe(false);
    expect(cues?.departure).toBeCloseTo(90, 0);
    expect(cues?.lengthMetres).toBeCloseTo(2000, -1);
  });

  // The case a painted line cannot answer at all: both markers land on one
  // coordinate, so without this flag one of the two ends is simply missing.
  it("reports a loop's two ends as the same place", () => {
    const cues = routeCues(square(2000));

    expect(cues?.sharedTerminal).toBe(true);
    // Left along the southern edge heading east, came back down the western
    // edge heading south. Both readings are still available, which is what keeps
    // start and finish distinguishable where the geometry cannot.
    expect(cues?.departure).toBeCloseTo(90, 0);
    expect(cues?.arrival).toBeCloseTo(180, 0);
  });

  it("reports an out-and-back's two ends as the same place", () => {
    const cues = routeCues(outAndBack(1000));

    expect(cues?.sharedTerminal).toBe(true);
    expect(cues?.departure).toBeCloseTo(90, 0);
    expect(cues?.arrival).toBeCloseTo(270, 0);
  });

  it("has nothing to say about a stage that is not a ride", () => {
    expect(routeCues([])).toBeNull();
    expect(routeCues([[8, 49]])).toBeNull();
    // Two points on the same spot: an end, but no direction to leave it in.
    expect(
      routeCues([
        [8, 49],
        [8, 49],
      ]),
    ).toBeNull();
  });

  it("measures a heading over enough ground to mean something", () => {
    // A first metre of satellite noise pointing north, on a route going east.
    const noisy: Position[] = [[8, 49], [8, 49.000009], ...eastward(21, 100).slice(1)];

    expect(routeCues(noisy)?.departure).toBeCloseTo(90, 0);
  });
});

describe("the description a reader hears", () => {
  it("says where the ride goes when the ends are apart", () => {
    const cues = routeCues(eastward(21, 100));

    expect(cues && cuesDescription(cues)).toBe(
      "Starts and finishes 2.0 km apart, the finish lying to the east. The ride leaves the start heading east.",
    );
  });

  it("measures the gap between the ends across the ground, not along the route", () => {
    // Three sides of a two-kilometre square: six kilometres ridden, but the
    // finish is one side away from the start.
    const wandering = square(2000).slice(0, 3 * (2000 / 50) + 1);
    const cues = routeCues(wandering);

    expect(cues?.lengthMetres).toBeCloseTo(6000, -2);
    expect(cues && cuesDescription(cues)).toBe(
      "Starts and finishes 2.0 km apart, the finish lying to the north. The ride leaves the start heading east.",
    );
  });

  it("says plainly that a loop comes back to where it started", () => {
    const cues = routeCues(square(2000));

    expect(cues && cuesDescription(cues)).toBe(
      "Starts and finishes at the same point. The ride leaves the start heading east and returns from the north, 8.0 km later.",
    );
  });
});

describe("direction chevrons", () => {
  const resolution = metresPerPixel(13, 49);

  it("points each chevron the way the route is ridden", () => {
    const chevrons = directionChevrons(eastward(201, 50), { metresPerPixel: resolution });

    expect(chevrons.length).toBeGreaterThan(1);
    for (const chevron of chevrons) {
      const [left, tip, right] = chevron as [Position, Position, Position];
      // Both arms trail behind the tip, which is what makes the shape point.
      expect(tip[0]).toBeGreaterThan(left[0]);
      expect(tip[0]).toBeGreaterThan(right[0]);
      // Symmetric about the direction of travel: one arm each side of the line.
      expect(left[1]).toBeLessThan(tip[1]);
      expect(right[1]).toBeGreaterThan(tip[1]);
    }
  });

  it("spaces them on screen, so length and zoom do not multiply them", () => {
    const spacingOf = (coordinates: Position[], zoom: number) => {
      const chevrons = directionChevrons(coordinates, {
        metresPerPixel: metresPerPixel(zoom, 49),
      });
      const tips = chevrons.map((chevron) => chevron[1] as Position);
      const gaps = tips.slice(1).map((tip, index) => haversineMetres(tips[index] as Position, tip));

      return { count: chevrons.length, gaps };
    };

    // A stage twenty times longer, framed at the zoom that fits it, gets the
    // same handful of cues rather than twenty times as many.
    const short = spacingOf(eastward(201, 50), 13);
    const long = spacingOf(eastward(201, 1000), 13 - Math.log2(20));
    expect(long.count).toBe(short.count);

    // And a pixel's worth of ground doubling doubles the ground between cues,
    // which is what keeps them the same distance apart on screen.
    const zoomedIn = spacingOf(eastward(201, 50), 14);
    const gap = (spacing: { gaps: number[] }) => spacing.gaps[0] ?? 0;
    expect(gap(short) / gap(zoomedIn)).toBeCloseTo(2, 1);
    expect(gap(short) / resolution).toBeCloseTo(CHEVRON_SPACING_PIXELS, 0);
  });

  it("never turns a route into a row of arrows", () => {
    // A route far longer than the window it is drawn in, where pixel spacing
    // alone would ask for hundreds of cues.
    const chevrons = directionChevrons(eastward(2001, 500), {
      metresPerPixel: metresPerPixel(13, 49),
    });

    expect(chevrons.length).toBeLessThanOrEqual(MAX_CHEVRONS);
    expect(chevrons.length).toBeGreaterThan(1);
  });

  it("leaves the terminals room, so no cue is drawn under a marker", () => {
    const coordinates = eastward(201, 50);
    const distances = cumulativeMetres(coordinates);
    const total = distances[distances.length - 1] ?? 0;
    const chevrons = directionChevrons(coordinates, { metresPerPixel: resolution });

    for (const chevron of chevrons) {
      const tip = chevron[1] as Position;
      const fromStart = haversineMetres(coordinates[0] as Position, tip);
      const fromFinish = haversineMetres(coordinates[coordinates.length - 1] as Position, tip);
      expect(fromStart).toBeGreaterThan(10 * resolution);
      expect(fromFinish).toBeGreaterThan(10 * resolution);
      expect(fromStart).toBeLessThan(total);
    }
  });

  it("draws no cue on a stage its own markers already cover", () => {
    // Forty metres end to end, at a zoom where that is a few pixels: the two
    // markers are touching, and a chevron between them would be under both.
    expect(directionChevrons(eastward(5, 10), { metresPerPixel: resolution })).toEqual([]);
  });

  it("follows a loop round its corners rather than cutting across them", () => {
    const coordinates = square(4000);
    const chevrons = directionChevrons(coordinates, { metresPerPixel: resolution });

    expect(chevrons.length).toBeGreaterThan(3);
    // Every tip sits on the drawn route: a cue placed off the line would be
    // pointing at ground the ride never covered.
    for (const chevron of chevrons) {
      const tip = chevron[1] as Position;
      const nearest = Math.min(...coordinates.map((position) => haversineMetres(position, tip)));
      expect(nearest).toBeLessThan(30);
    }
  });

  it("has nothing to draw without a ride or a camera", () => {
    expect(directionChevrons([[8, 49]], { metresPerPixel: resolution })).toEqual([]);
    expect(directionChevrons(eastward(21, 100), { metresPerPixel: 0 })).toEqual([]);
  });
});

describe("the positions a chevron is built from", () => {
  it("keeps a chevron flat where the route stores a point twice", () => {
    // A repeated point leaves a segment with no length to walk along, and the
    // stored points carry elevation. A chevron is map geometry either way.
    const elevated = eastward(41, 50).map(
      ([longitude, latitude]): Position => [longitude, latitude, 120],
    );
    const repeated = [...elevated.slice(0, 20), elevated[19] as Position, ...elevated.slice(20)];
    const chevrons = directionChevrons(repeated, { metresPerPixel: metresPerPixel(13, 49) });

    expect(chevrons.length).toBeGreaterThan(0);
    for (const chevron of chevrons) {
      for (const position of chevron) {
        expect(position).toHaveLength(2);
      }
    }
  });
});
