/**
 * The whole-rows math a fixed-height caller relies on to never show a row cut
 * in half.
 */

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Climb } from "../../lib/climbs";
import { ClimbsSidebar, rowsToShow } from "./ClimbsSidebar";

describe("rowsToShow", () => {
  it("subtracts the list's own top margin, not just the header's height", () => {
    // 59px left after the header: two 28px rows plus the list's 4px margin
    // would need 60px. Ignoring that margin claims the second row fits when
    // only 55px of it actually does.
    expect(rowsToShow(true, 59, 0)).toBe(1);
  });

  it("fits exactly the rows the margin and row height allow", () => {
    // 60px: header, then the list's 4px margin, then exactly two 28px rows.
    expect(rowsToShow(true, 60, 0)).toBe(2);
  });

  it("never reports zero, so a cramped panel still shows one row", () => {
    expect(rowsToShow(true, 10, 0)).toBe(1);
  });

  it("does not snap to whole rows outside fixed-height mode", () => {
    expect(rowsToShow(false, 200, 0)).toBeNull();
  });

  it("holds off until the section has actually been measured", () => {
    expect(rowsToShow(true, 0, 0)).toBeNull();
  });
});

function climb(attempts: [number, string][]): Climb {
  return {
    startMetres: 1000,
    endMetres: 1600,
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

describe("ClimbsSidebar times", () => {
  it("reads out the rider's quickest and most recent time over a climb", () => {
    // Served quickest first; the most recent is the later date, not the later
    // place in the list.
    const ridden = climb([
      [760, "2026-08-01T06:00:00Z"],
      [785, "2026-09-01T06:00:00Z"],
    ]);

    render(<ClimbsSidebar climbs={[ridden]} onSelect={() => {}} />);

    expect(screen.getByText("12:40 · 13:05")).toBeInTheDocument();
  });

  // Most climbs, for most riders, have never been ridden: an em dash would be
  // a figure-shaped thing where there is no figure.
  it("says nothing at all for a climb the rider has not ridden", () => {
    render(<ClimbsSidebar climbs={[climb([])]} onSelect={() => {}} />);

    // A time, not the separator: the column's own header carries one of those.
    expect(screen.queryByText(/\d+:\d\d/)).toBeNull();
  });
});
