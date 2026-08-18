import { describe, expect, it } from "vitest";
import { MIN_WINDOW_METRES, spanBetween, widened } from "./selection";

describe("spanBetween", () => {
  it("reads a stretch in the order it is ridden, whichever way it was drawn", () => {
    expect(spanBetween(400, 1200)).toEqual({ startMetres: 400, endMetres: 1200 });
    expect(spanBetween(1200, 400)).toEqual({ startMetres: 400, endMetres: 1200 });
  });
});

describe("widened", () => {
  it("leaves a stretch long enough to plot exactly as it was chosen", () => {
    expect(widened({ startMetres: 1000, endMetres: 4000 }, 10_000)).toEqual({
      startMetres: 1000,
      endMetres: 4000,
    });
  });

  it("grows a stretch too short to plot about its middle", () => {
    expect(widened({ startMetres: 5000, endMetres: 5040 }, 10_000)).toEqual({
      startMetres: 5020 - MIN_WINDOW_METRES / 2,
      endMetres: 5020 + MIN_WINDOW_METRES / 2,
    });
  });

  it("slides a grown stretch back inside the route rather than truncating it", () => {
    expect(widened({ startMetres: 0, endMetres: 30 }, 10_000)).toEqual({
      startMetres: 0,
      endMetres: MIN_WINDOW_METRES,
    });
    expect(widened({ startMetres: 9_980, endMetres: 10_000 }, 10_000)).toEqual({
      startMetres: 10_000 - MIN_WINDOW_METRES,
      endMetres: 10_000,
    });
  });

  it("asks no more of a route than it has, on one shorter than the minimum", () => {
    expect(widened({ startMetres: 40, endMetres: 60 }, 120)).toEqual({
      startMetres: 0,
      endMetres: 120,
    });
  });
});
