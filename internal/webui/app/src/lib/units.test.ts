import { describe, expect, it } from "vitest";
import { distanceLabel, distanceValue } from "./units";

describe("distanceValue", () => {
  it("reads a metre distance as kilometres", () => {
    expect(distanceValue(42_500)).toBeCloseTo(42.5, 6);
    expect(distanceValue(0)).toBe(0);
  });
});

describe("distanceLabel", () => {
  it("reads a whole-route step as whole kilometres", () => {
    expect(distanceLabel(42_000, 10)).toBe("42");
  });

  it("gains decimals as the step narrows", () => {
    expect(distanceLabel(1500, 0.5)).toBe("1.5");
    expect(distanceLabel(1234, 0.02)).toBe("1.23");
  });

  // Three is the ceiling: a metre of a kilometre is finer than any zoom the
  // chart offers, and a label reading nine digits tells neighbours nothing more.
  it("stops adding decimals at three, however fine the step", () => {
    expect(distanceLabel(1234.5678, 0.000_01)).toBe("1.235");
  });
});
