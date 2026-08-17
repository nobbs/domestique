import { describe, expect, it } from "vitest";
import { formatCount, formatDistance, formatTimestamp } from "./format";

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
