import { describe, expect, it } from "vitest";
import type { Position } from "../../api/types";
import { interpolateLegs, provisionalLegs, resample, routedLegs } from "./RouteTransition";

const a = { id: 0, longitude: 0, latitude: 0 };
const b = { id: 1, longitude: 1, latitude: 0 };
const c = { id: 2, longitude: 2, latitude: 0 };
const routed: Position[][] = [
  [
    [0, 0],
    [0.5, 0.2],
    [1, 0],
  ],
  [
    [1, 0],
    [1.5, -0.2],
    [2, 0],
  ],
];

describe("routedLegs", () => {
  it("cuts a line at the distance each waypoint is reached", () => {
    const line: Position[] = [
      [0, 0],
      [0.001, 0],
      [0.002, 0],
    ];
    const legs = routedLegs(line, [0, 74, 148]);

    expect(legs).toHaveLength(2);
    expect(legs?.[0]?.[0]).toEqual([0, 0]);
    expect(legs?.[1]?.at(-1)).toEqual([0.002, 0]);
    expect(routedLegs(line, undefined)).toBeNull();
  });
});

describe("provisionalLegs", () => {
  it("keeps routed legs whose ends stayed put and draws the rest straight", () => {
    expect(provisionalLegs([a, b, c], [a, b, c], routed, [])).toEqual([
      { coordinates: routed[0], provisional: false },
      { coordinates: routed[1], provisional: false },
    ]);

    const moved = { ...c, latitude: 1 };
    expect(provisionalLegs([a, b, moved], [a, b, c], routed, [])).toEqual([
      { coordinates: routed[0], provisional: false },
      {
        coordinates: [
          [1, 0],
          [2, 1],
        ],
        provisional: true,
      },
    ]);

    const inserted = { id: 3, longitude: 0.5, latitude: 1 };
    const legs = provisionalLegs([a, inserted, b, c], [a, b, c], routed, []);
    expect(legs.map((leg) => leg.provisional)).toEqual([true, true, false]);
  });

  it("draws a leg straight again once it is switched to or from a straight line", () => {
    const legs = provisionalLegs([a, { ...b, straight: true }, c], [a, b, c], routed, []);

    expect(legs.map((leg) => leg.provisional)).toEqual([true, false]);
  });

  it("stands a line it cannot cut whole until a waypoint moves", () => {
    const line: Position[] = [
      [0, 0],
      [2, 0],
    ];

    expect(provisionalLegs([a, c], [a, c], null, line)).toEqual([
      { coordinates: line, provisional: false },
    ]);
    expect(provisionalLegs([a, c], null, null, line)[0]?.provisional).toBe(true);
  });
});

describe("resample", () => {
  it("spreads points evenly along the line's length", () => {
    expect(
      resample(
        [
          [0, 0],
          [1, 0],
          [1, 3],
        ],
        5,
      ),
    ).toEqual([
      [0, 0],
      [1, 0],
      [1, 1],
      [1, 2],
      [1, 3],
    ]);
  });
});

describe("interpolateLegs", () => {
  it("moves each leg from its old shape to its new one", () => {
    const from = [
      {
        coordinates: [
          [0, 0],
          [2, 0],
        ] as Position[],
        provisional: true,
      },
    ];
    const to = [
      {
        coordinates: [
          [0, 0],
          [1, 2],
          [2, 0],
        ] as Position[],
        provisional: false,
      },
    ];

    expect(interpolateLegs(from, to, 0)[0]?.coordinates[1]?.[1]).toBeCloseTo(0);
    expect(interpolateLegs(from, to, 1)[0]?.coordinates[1]).toEqual([1, 2]);
    expect(interpolateLegs(from, to, 0.5)[0]).toMatchObject({ provisional: false });
  });
});
