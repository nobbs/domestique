import { describe, expect, it } from "vitest";
import { MIN_WIDTH, plotAxis } from "./plotAxis";

describe("plotAxis", () => {
  it("maps the start of the stretch to 0 and the end to the plot width", () => {
    const { plotWidth, x } = plotAxis(600, 1000, 5000);

    expect(x(1000)).toBeCloseTo(0);
    expect(x(5000)).toBeCloseTo(plotWidth);
  });

  it("is monotonic between the two ends", () => {
    const { x } = plotAxis(600, 0, 10_000);

    const pixels = [0, 2500, 5000, 7500, 10_000].map(x);
    for (let index = 1; index < pixels.length; index++) {
      expect(pixels[index]).toBeGreaterThan(pixels[index - 1] ?? Number.NEGATIVE_INFINITY);
    }
  });

  it("clamps a card narrower than the minimum to that minimum", () => {
    const narrow = plotAxis(50, 0, 1000);
    const atMinimum = plotAxis(MIN_WIDTH, 0, 1000);

    expect(narrow.plotWidth).toBe(atMinimum.plotWidth);
  });

  it("does not divide by zero for a stretch of no length", () => {
    const { x } = plotAxis(600, 500, 500);

    expect(Number.isFinite(x(500))).toBe(true);
  });
});
