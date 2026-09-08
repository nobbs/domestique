import { describe, expect, it } from "vitest";
import type { RouteClimb } from "../api/types";
import { climbTimes } from "./climbAttempts";
import type { Climb } from "./climbs";

function found(startMetres: number): Climb {
  return {
    startMetres,
    endMetres: startMetres + 600,
    distanceMetres: 600,
    ascentMetres: 36,
    averageGradePercent: 6,
    maxGradePercent: 8,
  };
}

function served(startMetres: number, attempts: [number, string][]): RouteClimb {
  return {
    startMetres,
    endMetres: startMetres + 600,
    distanceMetres: 600,
    ascentMetres: 36,
    averageGradePercent: 6,
    maxGradePercent: 8,
    attempts: attempts.map(([seconds, riddenAt], index) => ({
      activityId: index + 1,
      riddenAt,
      seconds,
      vamMetresPerHour: (36 / seconds) * 3600,
    })),
  };
}

describe("climbTimes", () => {
  it("reads the quickest attempt and the most recent one", () => {
    const times = climbTimes(
      [found(1000)],
      [
        served(1000, [
          [720, "2026-08-01T06:00:00Z"],
          [780, "2026-09-01T06:00:00Z"],
        ]),
      ],
    );

    expect(times.get(0)?.bestSeconds).toBe(720);
    expect(times.get(0)?.lastSeconds).toBe(780);
    expect(times.get(0)?.attempts).toHaveLength(2);
  });

  it("pairs by where a climb starts rather than by its place in the list", () => {
    const times = climbTimes(
      [found(2600), found(1000)],
      [served(1000, [[720, "2026-08-01T06:00:00Z"]])],
    );

    expect(times.has(0)).toBe(false);
    expect(times.get(1)?.bestSeconds).toBe(720);
  });

  // Two implementations of one rule: where they disagree about where a climb
  // begins, the climb carries no times rather than another climb's.
  it("leaves a climb the two put in different places without times", () => {
    const times = climbTimes([found(1000)], [served(1400, [[720, "2026-08-01T06:00:00Z"]])]);

    expect(times.size).toBe(0);
  });

  it("leaves a climb nobody has ridden without times", () => {
    expect(climbTimes([found(1000)], [served(1000, [])]).size).toBe(0);
  });
});
