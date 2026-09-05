import { describe, expect, it } from "vitest";
import { histogram } from "./histogram";

describe("histogram", () => {
  it("counts values into equal bins, with the edges going up and overflow into the last", () => {
    expect(histogram([0, 5, 10, 25, 99, 100, 250], 0, 100, 4)).toEqual([3, 1, 0, 3]);
  });

  it("answers no values with empty bins", () => {
    expect(histogram([], 0, 100, 3)).toEqual([0, 0, 0]);
  });

  it("leaves every bin empty over a domain with no width", () => {
    expect(histogram([1, 2], 5, 5, 3)).toEqual([0, 0, 0]);
    expect(histogram([1, 2], 5, 5, 0)).toEqual([]);
    expect(histogram([1, 2], 0, 10, 0)).toEqual([]);
  });
});
