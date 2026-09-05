import { describe, expect, it } from "vitest";
import { domainOf } from "./domain";

describe("domainOf", () => {
  it("rounds the largest value up to a whole step", () => {
    expect(domainOf([12_300, 47_800], [1_000, 5_000])).toEqual({ max: 48_000, step: 1_000 });
  });

  it("coarsens the step once a finer one would need too many stops", () => {
    expect(domainOf([180_000], [1_000, 5_000, 10_000])).toEqual({ max: 180_000, step: 5_000 });
  });

  it("gives an empty library one step of track", () => {
    expect(domainOf([], [100, 500])).toEqual({ max: 100, step: 100 });
  });

  it("survives a library too large to spread into arguments", () => {
    const values = new Array<number>(200_000).fill(1_000);
    values[123_456] = 61_500;
    expect(domainOf(values, [1_000, 2_000])).toEqual({ max: 62_000, step: 2_000 });
  });
});
