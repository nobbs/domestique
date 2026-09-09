import { describe, expect, it } from "vitest";
import type { Activity, ActivityRouteMatch } from "../api/types";
import { riddenOn } from "./rideHistory";

const ROUTE = { provider: "veloplanner", sourceRouteId: 12, stageOrder: 2 };

function match(overrides: Partial<ActivityRouteMatch> = {}): ActivityRouteMatch {
  return {
    ...ROUTE,
    routeCoverage: 1,
    rideCoverage: 1,
    direction: "forward",
    ...overrides,
  };
}

/** A ride of this route unless the overrides say it was ridden elsewhere. */
function ride(id: number, startedAt: string, overrides: Partial<Activity> = {}): Activity {
  return {
    id,
    startedAt,
    distanceMetres: 42_000,
    movingSeconds: 5_400,
    elapsedSeconds: 6_000,
    ascentMetres: 600,
    typeId: 40,
    locationId: 0,
    provider: "wahoo",
    routeMatch: match(),
    ...overrides,
  };
}

/** A ride matched to nothing: the field is absent rather than undefined. */
function unmatched(id: number, startedAt: string): Activity {
  const { routeMatch, ...rest } = ride(id, startedAt);

  return rest;
}

describe("riddenOn", () => {
  it("keeps only the rides matched to this route", () => {
    const rides = riddenOn(
      [
        ride(1, "2026-08-01T08:00:00Z"),
        ride(2, "2026-08-02T08:00:00Z", { routeMatch: match({ stageOrder: 3 }) }),
        unmatched(3, "2026-08-03T08:00:00Z"),
      ],
      ROUTE,
    );

    expect(rides.map((entry) => entry.ride.id)).toEqual([1]);
  });

  it("orders the history newest first, whatever order it was given in", () => {
    const rides = riddenOn(
      [
        ride(1, "2026-08-01T08:00:00Z"),
        ride(3, "2026-08-20T08:00:00Z"),
        ride(2, "2026-08-10T08:00:00Z"),
      ],
      ROUTE,
    );

    expect(rides.map((entry) => entry.ride.id)).toEqual([3, 2, 1]);
  });

  it("calls the fastest whole lap the best and reads its speed off its moving time", () => {
    const rides = riddenOn(
      [
        ride(1, "2026-08-01T08:00:00Z", { movingSeconds: 5_400 }),
        ride(2, "2026-08-10T08:00:00Z", { movingSeconds: 4_800 }),
      ],
      ROUTE,
    );

    expect(rides.filter((entry) => entry.personalBest).map((entry) => entry.ride.id)).toEqual([2]);
    expect(rides[0]?.speedKmh).toBeCloseTo(31.5, 1);
  });

  it("leaves a partial lap out of the best even where it was the quickest", () => {
    const rides = riddenOn(
      [
        ride(1, "2026-08-01T08:00:00Z", { movingSeconds: 5_400 }),
        ride(2, "2026-08-10T08:00:00Z", {
          movingSeconds: 3_600,
          routeMatch: match({ routeCoverage: 0.94 }),
        }),
      ],
      ROUTE,
    );

    expect(rides.find((entry) => entry.ride.id === 2)?.whole).toBe(false);
    expect(rides.filter((entry) => entry.personalBest).map((entry) => entry.ride.id)).toEqual([1]);
  });

  it("claims no best where every lap was partial, and no speed without moving time", () => {
    const rides = riddenOn(
      [
        ride(1, "2026-08-01T08:00:00Z", {
          movingSeconds: 0,
          routeMatch: match({ routeCoverage: 0.93 }),
        }),
      ],
      ROUTE,
    );

    expect(rides.some((entry) => entry.personalBest)).toBe(false);
    expect(rides[0]?.speedKmh).toBeNull();
  });

  it("is empty for a route nobody has ridden", () => {
    expect(riddenOn([], ROUTE)).toEqual([]);
  });
});
