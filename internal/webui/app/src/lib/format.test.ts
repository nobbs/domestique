import { describe, expect, it } from "vitest";
import { formatCount, formatDistance, formatElevation, formatTimestamp } from "./format";

describe("formatDistance", () => {
  it.each([
    [0, "—"],
    [-5, "—"],
    [Number.NaN, "—"],
    [420, "420 m"],
    [1500, "1.5 km"],
    [42_500, "42.5 km"],
    [180_400, "180 km"],
  ])("formats %p as %p", (metres, expected) => {
    expect(formatDistance(metres)).toBe(expected);
  });
});

describe("formatCount", () => {
  it("uses the singular for exactly one", () => {
    expect(formatCount(1, "point")).toBe("1 point");
  });

  it("uses the plural otherwise", () => {
    expect(formatCount(0, "point")).toBe("0 points");
    expect(formatCount(2, "point")).toBe("2 points");
  });
});

describe("formatTimestamp", () => {
  it("reports a missing timestamp plainly", () => {
    expect(formatTimestamp(undefined)).toBe("never");
  });

  it("reports an unparseable timestamp without throwing", () => {
    expect(formatTimestamp("not-a-date")).toBe("unknown");
  });

  it("renders a valid timestamp", () => {
    expect(formatTimestamp("2026-08-17T08:00:00Z")).not.toBe("unknown");
  });
});

describe("formatElevation", () => {
  /*
   * Not `formatDistance`: nought there means a route with nothing to measure and
   * reads as an em dash, but nought metres above sea level is the coast, and a
   * route that drops below it is a real one.
   */
  it.each([
    [0, "0 m"],
    [-4, "-4 m"],
    [960.4, "960 m"],
    [960.6, "961 m"],
  ])("formats %p as %p", (metres, expected) => {
    expect(formatElevation(metres)).toBe(expected);
  });

  it("says nothing rather than NaN for a height it does not have", () => {
    expect(formatElevation(Number.NaN)).toBe("—");
    expect(formatElevation(Number.POSITIVE_INFINITY)).toBe("—");
  });
});
