/**
 * The whole-rows math a fixed-height caller relies on to never show a row cut
 * in half.
 */

import { describe, expect, it } from "vitest";
import { rowsToShow } from "./ClimbsSidebar";

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
